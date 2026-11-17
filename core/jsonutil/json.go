// Package jsonutil 提供 Go 1.27 encoding/json/v2 的少量适配：
// 标准库 v2 不再提供 RawMessage、NewDecoder/NewEncoder 和 MarshalIndent，
// 这里用 jsontext 补齐这些能力，调用方统一走 v2。
package jsonutil

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"io"
)

// RawMessage 等价于 v1 的 json.RawMessage：保存一段未解析的 JSON 文本。
type RawMessage = jsontext.Value

// Decoder 包装 jsontext.Decoder，提供与 v1 json.Decoder 相同的 Decode 方法。
type Decoder struct {
	dec *jsontext.Decoder
}

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{dec: jsontext.NewDecoder(r)}
}

func (d *Decoder) Decode(v any) error {
	return jsonv2.UnmarshalDecode(d.dec, v)
}

// Encoder 提供与 v1 json.Encoder 相同的 Encode 方法（每条 JSON 后追加换行）。
// jsontext.Encoder 是单值状态机，连续 MarshalEncode 需要手动重置；helper 协议
// 每次只发一个小对象，这里用 jsonv2.Marshal 生成单值再写换行，行为与 v1 一致。
type Encoder struct {
	w io.Writer
}

func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

func (e *Encoder) Encode(v any) error {
	// 保留 v1 的 map 稳定顺序；helper 协议按行解析，稳定输出便于排查。
	b, err := jsonv2.Marshal(v, jsonv2.Deterministic(true))
	if err != nil {
		return err
	}
	if _, err := e.w.Write(b); err != nil {
		return err
	}
	_, err = io.WriteString(e.w, "\n")
	return err
}

// MarshalIndent 等价于 v1 json.MarshalIndent，但底层使用 jsontext 缩进编码。
func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	var buf bytes.Buffer
	opts := []jsontext.Options{jsontext.WithIndent(indent)}
	if prefix != "" {
		opts = append(opts, jsontext.WithIndentPrefix(prefix))
	}
	enc := jsontext.NewEncoder(&buf, opts...)
	if err := jsonv2.MarshalEncode(enc, v, jsonv2.Deterministic(true)); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}
