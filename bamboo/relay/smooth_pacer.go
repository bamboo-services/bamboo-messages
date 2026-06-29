package relay

import (
	"context"
	"time"

	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// pacerMode pacer 运行模式。
type pacerMode int

const (
	modeNormal pacerMode = iota // 正常流式 — EMA 自适应间隔
	modeDrain                   // 尾部加速 — 阶梯式递减间隔
	modeFlush                   // 错误冲刷 — 立即排空全部
)

// signalType 控制信号类型。
type signalType int

const (
	sigEnd signalType = iota + 1 // 上游已结束 → 切换到 DRAIN
)

// inputBufferSize input channel 缓冲大小。
// 128 足够吸收上游短时突发，避免 Push 阻塞；
// 极端情况下 select ctx.Done 保证可取消。
const inputBufferSize = 128

// minIntervalFloor 积压感知间隔缩减的下限。
//
// 当队列积压从 10 增长到 100 时，NORMAL 模式的有效间隔会
// 从 baseInterval 线性缩减到这个下限（2ms），避免过度积压。
// 到达下限后由 effectiveTokensPerFrame 接管，通过扩容 token 提升吞吐。
const minIntervalFloor = 2 * time.Millisecond

// SmoothPacer 流式平滑缓冲器。
//
// 接收 codec 序列化后的 SSE 帧，通过 FrameParser 切分为微帧，
// 按 EMA 自适应间隔匀速释放到输出 channel。
//
// 并发模型：
//   - Push 在 RelayStream goroutine 中调用
//   - run() 在独立 pacer goroutine 中运行
//   - 两者通过 input channel + signal channel 通信
//   - queue 只在 run goroutine 中操作（零竞争，不使用 sync.Mutex）
type SmoothPacer struct {
	params SmoothParams
	parser *FrameParser
	out    chan<- []byte
	ctx    context.Context

	input  chan []byte    // 数据输入（Push → run）
	signal chan signalType // 控制信号（SignalEnd → run）

	// pacer goroutine 内部状态（只在 run 中读写，无竞争）
	queue        []microFrame
	mode         pacerMode
	drainInitial int // DRAIN 模式初始队列长度

	// EMA 状态（只在 run 中读写）
	smoothedInterval time.Duration
	lastArrival      time.Time

	// 速率采样回调（只在 run 中调用）
	// 每次 outputBatch 输出帧后触发，按帧 kind 区分 thinking/output
	onRateSample func(elapsedSec, tokensPerSec float64, kind provider.RateSampleKind)
	startTime    time.Time
	lastEmitTime time.Time

	// 生命周期
	done chan struct{}
}

// NewSmoothPacer 创建平滑缓冲器并启动 pacer goroutine。
//
// format 用于初始化 FrameParser；params 控制节奏参数；
// out 为输出 channel（由调用方创建，调用方负责 close）；
// ctx 取消时 pacer 进入 FLUSH 模式排空后退出。
func NewSmoothPacer(format codec.FormatType, params SmoothParams, out chan<- []byte, ctx context.Context) *SmoothPacer {
	p := &SmoothPacer{
		params:           params,
		parser:           NewFrameParser(format),
		out:              out,
		ctx:              ctx,
		input:            make(chan []byte, inputBufferSize),
		signal:           make(chan signalType, 1),
		done:             make(chan struct{}),
		queue:            make([]microFrame, 0, 64),
		mode:             modeNormal,
		smoothedInterval: params.MinInterval,
		startTime:        time.Now(),
	}
	go p.run()
	return p
}

// SetRateSampleCallback 设置速率采样回调。
//
// 回调在 outputBatch 每次 tick 输出帧后触发（在 run goroutine 中调用），
// 参数为 (从流开始的经过秒数, 该 tick 的 token/s 速率, 采样类型)。
// 必须在 run goroutine 启动前（NewSmoothPacer 返回后立即）调用。
func (p *SmoothPacer) SetRateSampleCallback(fn func(elapsedSec, tokensPerSec float64, kind provider.RateSampleKind)) {
	p.onRateSample = fn
}

// Push 非阻塞推送 SSE 帧到 pacer。
//
// SSE 帧会通过 FrameParser 切分为微帧，入队等待按间隔释放。
// input channel 有 128 缓冲；极端情况 select ctx.Done 保证可取消。
// 必须在 SignalEnd 之前调用（SignalEnd 后 input 不会再被接收）。
func (p *SmoothPacer) Push(data []byte) {
	select {
	case p.input <- data:
	case <-p.ctx.Done():
	}
}

// SignalEnd 通知 pacer 上游已结束，切换到 DRAIN 模式。
//
// 调用后 pacer 会先 flush parser 的 pendingTail 残余，
// 然后按阶梯加速间隔排空队列，排空后自动退出。
// 只能调用一次（signal channel 缓冲为 1，重复调用安全但无意义）。
func (p *SmoothPacer) SignalEnd() {
	select {
	case p.signal <- sigEnd:
	case <-p.ctx.Done():
	}
}

// Wait 等待 pacer goroutine 完全退出。
//
// 退出条件：DRAIN 模式队列排空 / ctx 取消后 FLUSH 排空 / 队列空且上游已结束。
// 调用方在 Wait 返回后可以安全 close(out)。
func (p *SmoothPacer) Wait() {
	<-p.done
}

// Close 清理资源（等同于 Wait，语义别名）。
//
// 通常由 Wait 隐式完成；显式调用 Close 确保资源释放。
func (p *SmoothPacer) Close() {
	<-p.done
}

// ── 核心 goroutine ──

// run pacer 核心循环 — 三模式状态机。
//
// 状态转换：
//
//	NORMAL ──SignalEnd──→ DRAIN ──queue空──→ exit
//	  │                     │
//	  └──ctx取消──→ FLUSH ──排空──→ exit
//	                     ↑
//	            DRAIN + ctx取消
//
// 输入阶段（!upstreamDone）：select input/signal/ctx.Done
// 输出阶段（有数据）：timer.Reset(interval) → select timer.C/input/signal/ctx.Done
func (p *SmoothPacer) run() {
	defer close(p.done)

	// panic 时不调用 flushAll — panic 说明内部状态已破坏，
	// 且 out channel 可能已被调用方关闭，继续写入会二次 panic。
	defer func() {
		recover()
	}()

	timer := time.NewTimer(0)
	defer timer.Stop()

	if !timer.Stop() {
		<-timer.C
	}

	upstreamDone := false
	timerActive := false

	for {
		// ── 检查完成条件 ──
		if upstreamDone && len(p.queue) == 0 {
			return
		}

		// ── 输入阶段：如果队列空且未结束，等待新数据 ──
		if !upstreamDone && len(p.queue) == 0 {
			select {
			case data := <-p.input:
				p.handleInput(data)
				continue
			case <-p.signal:
				p.enterDrain(&upstreamDone)
				continue
			case <-p.ctx.Done():
				p.enterFlushAndExit()
				return
			}
		}

		// ── 输出阶段：按模式确定间隔，等待 tick 或新事件 ──
		interval := p.currentInterval()

		// FLUSH 模式：interval == 0，立即排空
		if p.mode == modeFlush {
			p.flushAll()
			return
		}

		// 积压感知：仅当 timer 未激活且队列非空时才 Reset
		// 避免 input 到达后反复重置 timer 导致 outputBatch 饥饿（活锁）
		if !timerActive && len(p.queue) > 0 {
			timer.Reset(interval)
			timerActive = true
		}

		select {
		case <-timer.C:
			timerActive = false
			p.outputBatch()

		case data := <-p.input:
			// tick 期间有新数据到达，仅入队，不停止 timer
			// timer 会继续倒计时，下一轮 select 自然竞争
			p.handleInput(data)

		case <-p.signal:
			p.enterDrain(&upstreamDone)
			if timerActive {
				if !timer.Stop() {
					<-timer.C
				}
				timerActive = false
			}

		case <-p.ctx.Done():
			if timerActive {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timerActive = false
			}
			p.enterFlushAndExit()
			return
		}
	}
}

// ── 模式切换 ──

// enterDrain 切换到 DRAIN 模式（上游结束）。
//
// 记录初始队列长度用于阶梯加速比例计算，
// 并 flush parser 的 pendingTail 残余帧入队。
func (p *SmoothPacer) enterDrain(upstreamDone *bool) {
	*upstreamDone = true
	p.mode = modeDrain
	p.drainInitial = len(p.queue)
	p.flushParserTails()
}

// enterFlushAndExit 切换到 FLUSH 模式并立即排空（ctx 取消）。
func (p *SmoothPacer) enterFlushAndExit() {
	p.mode = modeFlush
	p.flushParserTails()
	p.flushAll()
}

// ── 数据处理 ──

// handleInput 处理一帧 SSE 数据。
//
// 更新 EMA（上游有数据到达时），通过 FrameParser 解析为微帧并入队。
// 只在 run goroutine 中调用（queue/parser 无竞争）。
func (p *SmoothPacer) handleInput(data []byte) {
	now := time.Now()
	if !p.lastArrival.IsZero() {
		upstreamInterval := now.Sub(p.lastArrival)
		alpha := p.params.EMAAlpha
		p.smoothedInterval = time.Duration(
			float64(upstreamInterval)*alpha + float64(p.smoothedInterval)*(1-alpha),
		)
	}
	p.lastArrival = now

	frames := p.parser.Parse(data)
	p.queue = append(p.queue, frames...)
}

// flushParserTails flush FrameParser 的 pendingTail 残余帧入队。
//
// 在 DRAIN 或 FLUSH 进入时调用，确保最后一帧的不完整 token 被输出。
func (p *SmoothPacer) flushParserTails() {
	tails := p.parser.FlushRemaining()
	p.queue = append(p.queue, tails...)
}

// ── 节奏控制 ──

// effectiveInterval 阶段一：积压感知间隔缩减。
//
// queueLen 10→100 映射到 factor 1.0→0.0（线性），
// effective 从 baseInterval 线性缩减到 minFloor。
// queueLen ≤ 10 时返回 baseInterval（无积压感知）。
func effectiveInterval(queueLen int, baseInterval time.Duration, minFloor time.Duration) time.Duration {
	if baseInterval < minFloor {
		baseInterval = minFloor
	}
	if queueLen <= 10 {
		return baseInterval
	}
	progress := float64(queueLen-10) / 90.0
	if progress > 1.0 {
		progress = 1.0
	}
	factor := 1.0 - progress
	effective := time.Duration(float64(baseInterval) * factor)
	if effective < minFloor {
		effective = minFloor
	}
	return effective
}

// effectiveTokensPerFrame 阶段二：interval 到 floor 后 token 扩容。
//
// 仅当 intervalAtFloor=true 且 queueLen > 20 时扩容。
// 每 20 帧额外积压 → multiplier +1，上限 8。
func effectiveTokensPerFrame(queueLen int, baseTokens int, intervalAtFloor bool) int {
	if !intervalAtFloor || queueLen <= 20 {
		return baseTokens
	}
	multiplier := 1 + (queueLen-20)/20
	if multiplier > 8 {
		multiplier = 8
	}
	return baseTokens * multiplier
}

// effectiveDrainInterval 计算 DRAIN 模式下的输出间隔。
//
// DRAIN 模式简化为两级：
//   - remaining > drainTier2Ratio → minIntervalFloor（快速排空但有间隔）
//   - remaining ≤ drainTier2Ratio → 0（立即排空）
//
// drainInitial == 0 时返回 minIntervalFloor（防止除零）。
func effectiveDrainInterval(queueLen int, drainInitial int, drainTier2Ratio float64) time.Duration {
	if drainInitial == 0 {
		return minIntervalFloor
	}
	remaining := float64(queueLen) / float64(drainInitial)
	if remaining > drainTier2Ratio {
		return minIntervalFloor
	}
	return 0
}

// currentInterval 根据当前模式返回输出间隔。
//
//	NORMAL: EMA 钳制到 [MinInterval, MaxInterval]
//	DRAIN:  阶梯式加速（按剩余比例分 Tier）
//	FLUSH:  0（立即排空，不等待）
func (p *SmoothPacer) currentInterval() time.Duration {
	switch p.mode {
	case modeNormal:
		baseInterval := p.smoothedInterval
		if baseInterval < p.params.MinInterval {
			baseInterval = p.params.MinInterval
		}
		if baseInterval > p.params.MaxInterval {
			baseInterval = p.params.MaxInterval
		}
		return effectiveInterval(len(p.queue), baseInterval, minIntervalFloor)

	case modeDrain:
		return effectiveDrainInterval(len(p.queue), p.drainInitial, p.params.DrainTier2Ratio)

	case modeFlush:
		return 0
	}

	return p.params.MinInterval
}

// ── 输出 ──

// outputBatch 从队列中取出一批微帧并输出到 out channel。
//
// Barrier 帧语义：遇到 barrier 时，先排空前面所有积压的数据帧，
// 然后输出 barrier 本身（保证时序：barrier 前的数据必须先到达）。
//
// 正常帧：每 tick 输出 TokensPerFrame 个 token（遇到 barrier 停止）。
func (p *SmoothPacer) outputBatch() {
	if len(p.queue) == 0 {
		return
	}

	// 队首是 barrier → 排空前面积压的数据帧后输出 barrier
	if p.queue[0].isBarrier {
		var thinkingN, outputN, toolN int
		for len(p.queue) > 0 && !p.queue[0].isBarrier {
			p.recordFrameForSampling(p.queue[0], &thinkingN, &outputN, &toolN)
			p.outputFrame(p.queue[0])
			p.queue = p.queue[1:]
		}
		p.emitRateSamples(thinkingN, outputN, toolN)
		if len(p.queue) > 0 {
			p.outputFrame(p.queue[0])
			p.queue = p.queue[1:]
		}
		return
	}

	// 正常输出：动态计算 TokensPerFrame（积压感知扩容）
	intervalAtFloor := p.currentInterval() == minIntervalFloor
	effectiveTokens := effectiveTokensPerFrame(len(p.queue), p.params.TokensPerFrame, intervalAtFloor)

	var thinkingN, outputN, toolN int
	tokensOutput := 0
	for len(p.queue) > 0 && tokensOutput < effectiveTokens {
		frame := p.queue[0]
		if frame.isBarrier {
			break
		}
		p.recordFrameForSampling(frame, &thinkingN, &outputN, &toolN)
		p.outputFrame(frame)
		p.queue = p.queue[1:]
		tokensOutput++
	}

	p.emitRateSamples(thinkingN, outputN, toolN)
}

func (p *SmoothPacer) recordFrameForSampling(frame microFrame, thinkingN, outputN, toolN *int) {
	switch frame.kind {
	case frameThinking:
		*thinkingN++
	case frameText:
		*outputN++
	case frameTool:
		*toolN++
	}
}

func (p *SmoothPacer) emitRateSamples(thinkingN, outputN, toolN int) {
	if p.onRateSample == nil {
		return
	}

	now := time.Now()
	if p.lastEmitTime.IsZero() {
		p.lastEmitTime = now
		return
	}

	intervalSec := now.Sub(p.lastEmitTime).Seconds()
	elapsedSec := now.Sub(p.startTime).Seconds()

	if intervalSec <= 0 {
		return
	}

	if thinkingN > 0 {
		rate := float64(thinkingN) / intervalSec
		p.onRateSample(elapsedSec, rate, provider.RateSampleKindThinking)
	}
	if outputN > 0 {
		rate := float64(outputN) / intervalSec
		p.onRateSample(elapsedSec, rate, provider.RateSampleKindOutput)
	}
	if toolN > 0 {
		rate := float64(toolN) / intervalSec
		p.onRateSample(elapsedSec, rate, provider.RateSampleKindTool)
	}

	p.lastEmitTime = now
}

// safeSend 向 out channel 安全发送数据，防止 send on closed channel panic。
// 通过 defer recover 捕获 — 因为 select 无法检测 channel 是否已关闭。
func (p *SmoothPacer) safeSend(data []byte) (sent bool) {
	defer func() {
		if r := recover(); r != nil {
			sent = false
		}
	}()
	select {
	case p.out <- data:
		return true
	case <-p.ctx.Done():
		return false
	}
}

// outputFrame 输出单个微帧到 out channel。
// 尊重 ctx 取消（取消时跳过，由调用方后续处理）。
func (p *SmoothPacer) outputFrame(frame microFrame) {
	p.safeSend(frame.data)
}

// flushAll 立即排空全部队列（FLUSH 模式或 ctx 取消时调用）。
//
// 使用独立 100ms timeout 替代 ctx.Done() — ctx 取消后仍尝试短暂排空队列，
// 避免上游已产生的数据被静默丢弃。100ms 足以排空 buffer 中的突发数据，
// 不会造成可感知的消费者延迟。
const flushAllTimeout = 100 * time.Millisecond

func (p *SmoothPacer) flushAll() {
	for len(p.queue) > 0 {
		if !p.flushSend(p.queue[0].data) {
			return
		}
		p.queue = p.queue[1:]
	}
}

// flushSend 用于 FLUSH 模式的安全写入，独立于 safeSend。
// FLUSH 时 ctx 通常已取消，safeSend 的 ctx.Done 分支会立即返回 false，
// 但 FLUSH 的语义是"尽快排空"，因此使用独立 timeout 而非 ctx。
func (p *SmoothPacer) flushSend(data []byte) (sent bool) {
	defer func() {
		if r := recover(); r != nil {
			sent = false
		}
	}()
	timer := time.NewTimer(flushAllTimeout)
	defer timer.Stop()
	select {
	case p.out <- data:
		return true
	case <-timer.C:
		return false
	}
}
