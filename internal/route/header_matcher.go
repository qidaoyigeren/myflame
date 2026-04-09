package route

import (
	"net/http"
	"regexp"
)

// HeaderMatcher 基于请求头的路由匹配条件
// 例：只有 Content-Type 为 application/json 时才匹配
type HeaderMatcher struct {
	matched map[string]*regexp.Regexp // 请求头名 → 正则
}

func NewHeaderMatcher(matched map[string]*regexp.Regexp) *HeaderMatcher {
	return &HeaderMatcher{
		matched: matched,
	}
}

// Match 返回 true 当所有 matches 都满足时
func (m *HeaderMatcher) Match(header http.Header) bool {
	for name, re := range m.matched {
		v := header.Get(name)
		if v == "" {
			return false
		}
		if !re.MatchString(v) {
			return false
		}
	}
	return true
}
