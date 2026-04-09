package route

import (
	"bytes"
	"sync"

	"github.com/alecthomas/participle/v2/lexer"
)

/*
路由引擎的三层架构
路由字符串 "/users/{id: /[0-9]+/}"
     ↓ Parser 解析
  Route AST（语法树）
     ↓ AddRoute 构建
  Tree（路由树，存储所有路由）
     ↓ Match 匹配
  Leaf（叶子节点，关联 Handler）
*/

// BindParameterValue 绑定参数的值：字面量或正则
type BindParameterValue struct {
	Literal *string `parser:"  @Ident"`         // 字面量，如 "**"
	Regex   *string `parser:"| '/' @Regex '/'"` // 正则，如 "/[0-9]+/"
}

// BindParameter 一个绑定参数对：名称 + 值
// 例：`id: /[0-9]+/` 或 `path: **`
type BindParameter struct {
	Ident string             `parser:"@Ident ':' ' '*"` // 参数名
	Value BindParameterValue `parser:"@@"`              // 参数值
}

// BindParameters 多个绑定参数（用逗号分隔）
// 例：`{name: /[a-z]+/, age: /[0-9]+/}`
type BindParameters struct {
	Parameters []BindParameter `parser:"( @@ ( ',' ' '* @@ )* )+"`
}

// SegmentElement 一个路由段内的单个元素
// 三种可能：纯文字 | {name} 占位符 | {name: value} 绑定参数
type SegmentElement struct {
	Pos            lexer.Position
	EndPos         lexer.Position
	Ident          *string         `parser:"  @Ident"`
	BindIdent      *string         `parser:"| '{' @Ident '}'"`
	BindParameters *BindParameters `parser:"| '{' @@ '}'"`
}

// Segment 路由的一个「段」，对应 URL 中两个 / 之间的部分
// 例：`/users`、`/{id}`、`/?optional`
type Segment struct {
	Pos      lexer.Position
	Slash    string           `parser:"'/'"`   // 开头的斜杠
	Optional bool             `parser:"@'?'?"` // ? 表示可选
	Elements []SegmentElement `parser:"@@*"`   // 段内元素列表

	// String() 的结果缓存（延迟计算）
	strOnce sync.Once
	str     string
}

// String 重建该 Segment 的字符串表示
func (s *Segment) String() string {
	s.strOnce.Do(func() {
		var buf bytes.Buffer
		buf.WriteString("/")
		if s.Optional {
			buf.WriteString("?")
		}
		for _, e := range s.Elements {
			if e.Ident != nil {
				buf.WriteString(*e.Ident)
			} else if e.BindIdent != nil {
				buf.WriteString("{")
				buf.WriteString(*e.BindIdent)
				buf.WriteString("}")
			} else if e.BindParameters != nil {
				buf.WriteString("{")
				for i, p := range e.BindParameters.Parameters {
					buf.WriteString(p.Ident)
					buf.WriteString(": ")
					if p.Value.Literal != nil {
						buf.WriteString(*p.Value.Literal)
					} else if p.Value.Regex != nil {
						buf.WriteString("/")
						buf.WriteString(*p.Value.Regex)
						buf.WriteString("/")
					}
					if i < len(e.BindParameters.Parameters)-1 {
						buf.WriteString(", ")
					}
				}
				buf.WriteString("}")
			}
		}
		s.str = buf.String()
	})
	return s.str
}

// Route 完整路由，由多个 Segment 组成
type Route struct {
	Segments []*Segment `parser:"@@+"` // 至少一个 Segment

	strOnce sync.Once
	str     string
}

func (r *Route) String() string {
	r.strOnce.Do(func() {
		var buf bytes.Buffer
		for _, s := range r.Segments {
			buf.WriteString(s.String())
		}
		r.str = buf.String()
	})
	return r.str
}
