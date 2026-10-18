// Package id 生成 ULID（Crockford base32，26 字符，按时间字典序有序）。
//
// 自己实现而不引依赖：只需要 30 行，且能保证"ID 可排序"这一存储层依赖的性质
// （见 04-技术路线/数据模型 §3：id TEXT PRIMARY KEY —— ULID 有序、可排序）。
package id

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
)

const (
	// crockford 是 Crockford Base32 字母表（去掉 I L O U，避免歧义）。
	crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	// encodedLen 是 ULID 的固定长度：128 bit（48 时间 + 80 随机）编码为 26 字符。
	encodedLen = 26
	// 26 字符 × 5 bit = 130 bit，比 128 bit 多 2 bit，故左侧补 2 个 0。
	padBits = 2
)

// New 生成一个 ULID。时间戳精度毫秒；同一毫秒内随机部分不保证单调。
func New() string {
	var raw [16]byte
	ms := uint64(time.Now().UnixMilli())
	// 48 bit 大端时间戳
	raw[0] = byte(ms >> 40)
	raw[1] = byte(ms >> 32)
	raw[2] = byte(ms >> 24)
	raw[3] = byte(ms >> 16)
	raw[4] = byte(ms >> 8)
	raw[5] = byte(ms)
	if _, err := rand.Read(raw[6:]); err != nil {
		// crypto/rand 失败意味着系统熵源坏了，这里只可能发生在极端环境；
		// 用时间戳填充以降级（仍然唯一性较弱，但不 panic 掉整个服务）。
		binary.BigEndian.PutUint64(raw[8:], uint64(time.Now().UnixNano()))
	}
	return encode(raw)
}

// Time 从 ULID 里解出毫秒时间戳；ID 非法时返回零值与错误。
func Time(s string) (time.Time, error) {
	if len(s) != encodedLen {
		return time.Time{}, fmt.Errorf("id: invalid ulid length %d", len(s))
	}
	var ms uint64
	var bits int
	for i := 0; i < 10; i++ {
		v, ok := decodeChar(s[i])
		if !ok {
			return time.Time{}, fmt.Errorf("id: invalid char %q", s[i])
		}
		bits += 5
		ms = ms<<5 | uint64(v)
	}
	// 前 10 字符 = 50 bit，高 2 bit 是补零，取低 48 bit
	return time.UnixMilli(int64(ms & 0xFFFFFFFFFFFF)).UTC(), nil
}

func encode(raw [16]byte) string {
	out := make([]byte, encodedLen)
	for i := 0; i < encodedLen; i++ {
		var v byte
		for b := 0; b < 5; b++ {
			bit := i*5 + b - padBits
			if bit < 0 {
				v <<= 1
				continue
			}
			byteIdx := bit / 8
			shift := uint(7 - bit%8)
			v = v<<1 | (raw[byteIdx]>>shift)&1
		}
		out[i] = crockford[v]
	}
	return string(out)
}

func decodeChar(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'A' && c <= 'Z':
		// 跳过 I L O U
		idx := -1
		for i := 10; i < len(crockford); i++ {
			if crockford[i] == c {
				idx = i
				break
			}
		}
		if idx < 0 {
			return 0, false
		}
		return byte(idx), true
	}
	return 0, false
}
