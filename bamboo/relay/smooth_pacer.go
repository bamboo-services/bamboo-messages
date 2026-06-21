package relay

import (
	"context"
	"time"

	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
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
		smoothedInterval: params.MinInterval, // 初始 = 最小间隔（首帧快速输出）
	}
	go p.run()
	return p
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

	timer := time.NewTimer(0)
	defer timer.Stop()

	// 清理可能残留的初始计时（NewTimer(0) 已触发）
	if !timer.Stop() {
		<-timer.C
	}

	upstreamDone := false

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

		// 首帧无延迟（smoothedInterval 初始 = MinInterval，但首帧应立即输出）
		// 通过 timer.Reset(interval) 控制节奏
		timer.Reset(interval)

		select {
		case <-timer.C:
			p.outputBatch()

		case data := <-p.input:
			// tick 期间有新数据到达，先处理数据，不重置 timer
			// timer 会继续倒计时，下一轮循环再处理输出
			p.handleInput(data)
			// 消耗掉未触发的 timer（避免下一轮立即触发）
			if !timer.Stop() {
				<-timer.C
			}

		case <-p.signal:
			p.enterDrain(&upstreamDone)
			if !timer.Stop() {
				<-timer.C
			}

		case <-p.ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
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

// currentInterval 根据当前模式返回输出间隔。
//
//	NORMAL: EMA 钳制到 [MinInterval, MaxInterval]
//	DRAIN:  阶梯式加速（按剩余比例分 Tier）
//	FLUSH:  0（立即排空，不等待）
func (p *SmoothPacer) currentInterval() time.Duration {
	switch p.mode {
	case modeNormal:
		interval := p.smoothedInterval
		if interval < p.params.MinInterval {
			interval = p.params.MinInterval
		}
		if interval > p.params.MaxInterval {
			interval = p.params.MaxInterval
		}
		return interval

	case modeDrain:
		if p.drainInitial == 0 {
			return p.params.MinInterval
		}
		remaining := float64(len(p.queue)) / float64(p.drainInitial)

		baseInterval := p.smoothedInterval
		if baseInterval < p.params.MinInterval {
			baseInterval = p.params.MinInterval
		}

		switch {
		case remaining > p.params.DrainTier1Ratio:
			return baseInterval
		case remaining > p.params.DrainTier2Ratio:
			return time.Duration(float64(baseInterval) * p.params.DrainTier1Mult)
		default:
			return time.Duration(float64(baseInterval) * p.params.DrainTier2Mult)
		}

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
		for len(p.queue) > 0 && !p.queue[0].isBarrier {
			p.outputFrame(p.queue[0])
			p.queue = p.queue[1:]
		}
		if len(p.queue) > 0 {
			p.outputFrame(p.queue[0])
			p.queue = p.queue[1:]
		}
		return
	}

	// 正常输出：TokensPerFrame 个 token
	tokensOutput := 0
	for len(p.queue) > 0 && tokensOutput < p.params.TokensPerFrame {
		frame := p.queue[0]
		if frame.isBarrier {
			break
		}
		p.outputFrame(frame)
		p.queue = p.queue[1:]
		tokensOutput++
	}
}

// outputFrame 输出单个微帧到 out channel。
// 尊重 ctx 取消（取消时跳过，由调用方后续处理）。
func (p *SmoothPacer) outputFrame(frame microFrame) {
	select {
	case p.out <- frame.data:
	case <-p.ctx.Done():
	}
}

// flushAll 立即排空全部队列（FLUSH 模式或 ctx 取消时调用）。
func (p *SmoothPacer) flushAll() {
	for len(p.queue) > 0 {
		select {
		case p.out <- p.queue[0].data:
			p.queue = p.queue[1:]
		case <-p.ctx.Done():
			return
		}
	}
}
