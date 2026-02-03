package main

import "context"

type loopTraceKey struct{}

// LoopTrace 扣子罗盘 trace 业务侧元数据，由业务层设置、data 层读取并上报
type LoopTrace struct {
	SpanName  string // 对应 span 名称，如 promptType
	ThreadID  string // 如 sessionId
	UserID    string // 用户ID
	MessageID string // 消息ID，可选
}

// WithLoopTrace 将 spanName/threadID/userID 写入 context，供 createArkResponse trace 上报使用
func WithLoopTrace(ctx context.Context, spanName, threadID, userID string) context.Context {
	return context.WithValue(ctx, loopTraceKey{}, &LoopTrace{
		SpanName: spanName,
		ThreadID: threadID,
		UserID:   userID,
	})
}

// getLoopTrace 内部使用，返回只读副本或空结构（避免返回 nil 时调用方要判空）
func getLoopTrace(ctx context.Context) LoopTrace {
	if t := GetLoopTrace(ctx); t != nil {
		return *t
	}
	return LoopTrace{}
}

// SetUserID 仅设置 UserID，保留 context 中已有的其它 LoopTrace 字段
func SetUserID(ctx context.Context, userID string) context.Context {
	t := getLoopTrace(ctx)
	t.UserID = userID
	return context.WithValue(ctx, loopTraceKey{}, &t)
}

// SetThreadID 仅设置 ThreadID，保留 context 中已有的其它 LoopTrace 字段
func SetThreadID(ctx context.Context, threadID string) context.Context {
	t := getLoopTrace(ctx)
	t.ThreadID = threadID
	return context.WithValue(ctx, loopTraceKey{}, &t)
}

// SetSpanName 仅设置 SpanName，保留 context 中已有的其它 LoopTrace 字段
func SetSpanName(ctx context.Context, spanName string) context.Context {
	t := getLoopTrace(ctx)
	t.SpanName = spanName
	return context.WithValue(ctx, loopTraceKey{}, &t)
}

// SetMessageID 仅设置 MessageID，保留 context 中已有的其它 LoopTrace 字段
func SetMessageID(ctx context.Context, messageID string) context.Context {
	t := getLoopTrace(ctx)
	t.MessageID = messageID
	return context.WithValue(ctx, loopTraceKey{}, &t)
}

// GetLoopTrace 从 context 读取扣子罗盘 trace 元数据，未设置时返回 nil
func GetLoopTrace(ctx context.Context) *LoopTrace {
	v := ctx.Value(loopTraceKey{})
	if v == nil {
		return nil
	}
	t, _ := v.(*LoopTrace)
	return t
}
