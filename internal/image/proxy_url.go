package image

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

const defaultImageProxyTTL = 24 * time.Hour

// imageProxySecret 进程级随机密钥,用于 HMAC 签名图片 URL。
// 进程重启后旧的签名 URL 全部失效,这是故意的(防止长期有效的 URL 泄漏)。
var imageProxySecret []byte

// proxyURLBuilder 允许 main.go 在需要时覆写代理 path 的生成方式。
// 未注入时回退到本包内置的签名实现。
var proxyURLBuilder atomic.Value

func init() {
	imageProxySecret = make([]byte, 32)
	if _, err := rand.Read(imageProxySecret); err != nil {
		for i := range imageProxySecret {
			imageProxySecret[i] = byte(i*31 + 7)
		}
	}
}

// SetProxyURLBuilder 注入代理 URL 构造函数。多次调用以最后一次为准。
func SetProxyURLBuilder(fn func(taskID string, idx int) string) {
	if fn == nil {
		return
	}
	proxyURLBuilder.Store(fn)
}

// BuildProxyURL 生成代理 URL。返回绝对 path(不含 host)。
func BuildProxyURL(taskID string, idx int, ttl time.Duration) string {
	if ttl <= 0 {
		ttl = defaultImageProxyTTL
	}
	expMs := time.Now().Add(ttl).UnixMilli()
	sig := computeImgSig(taskID, idx, expMs)
	return fmt.Sprintf("/p/img/%s/%d?exp=%d&sig=%s", taskID, idx, expMs, sig)
}

// BuildProxyURLs 生成一组本地代理 URL,长度与输入保持一致。
func BuildProxyURLs(taskID string, raw []string) []string {
	out := make([]string, len(raw))
	for i := range raw {
		out[i] = buildProxyPath(taskID, i)
	}
	return out
}

// BuildPublicProxyURL 在相对代理路径基础上按需补全 host。
// apiBaseURL 允许填写完整的 /v1 地址,这里只提取 scheme://host[:port]。
func BuildPublicProxyURL(apiBaseURL, taskID string, idx int, ttl time.Duration) string {
	return WithPublicBaseURL(BuildProxyURL(taskID, idx, ttl), apiBaseURL)
}

// BuildPublicImageURLs 优先按 fileIDs 数量生成可长期回放的代理 URL。
// 对于缺 file_ids 的历史数据,回落到原始 result_urls,避免直接返回空数组。
func BuildPublicImageURLs(apiBaseURL, taskID string, fileIDs, raw []string) []string {
	count := len(raw)
	if len(fileIDs) > count {
		count = len(fileIDs)
	}
	if count == 0 {
		return nil
	}

	out := make([]string, 0, count)
	for i := 0; i < count; i++ {
		if i < len(fileIDs) && strings.TrimSpace(fileIDs[i]) != "" {
			out = append(out, WithPublicBaseURL(buildProxyPath(taskID, i), apiBaseURL))
			continue
		}
		if i < len(raw) {
			out = append(out, WithPublicBaseURL(raw[i], apiBaseURL))
		}
	}
	return out
}

// WithPublicBaseURL 为相对 URL 补全对外 host。只取 apiBaseURL 的 origin,忽略其 path/query。
func WithPublicBaseURL(rawURL, apiBaseURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if u, err := url.Parse(rawURL); err == nil && u.IsAbs() {
		return rawURL
	}

	base, err := url.Parse(strings.TrimSpace(apiBaseURL))
	if err != nil || base == nil || base.Scheme == "" || base.Host == "" {
		return rawURL
	}
	origin := base.Scheme + "://" + base.Host
	if strings.HasPrefix(rawURL, "/") {
		return origin + rawURL
	}
	return origin + "/" + rawURL
}

// ComputeImgSig 计算图片 URL 签名（供 gateway 验证使用）。
func ComputeImgSig(taskID string, idx int, expMs int64) string {
	return computeImgSig(taskID, idx, expMs)
}

func computeImgSig(taskID string, idx int, expMs int64) string {
	mac := hmac.New(sha256.New, imageProxySecret)
	fmt.Fprintf(mac, "%s|%d|%d", taskID, idx, expMs)
	return hex.EncodeToString(mac.Sum(nil))[:24]
}

// VerifyImgSig 验证图片 URL 签名。
func VerifyImgSig(taskID string, idx int, expMs int64, sig string) bool {
	if expMs < time.Now().UnixMilli() {
		return false
	}
	want := computeImgSig(taskID, idx, expMs)
	return hmac.Equal([]byte(sig), []byte(want))
}

func buildProxyPath(taskID string, idx int) string {
	if v := proxyURLBuilder.Load(); v != nil {
		if fn, ok := v.(func(taskID string, idx int) string); ok && fn != nil {
			if out := strings.TrimSpace(fn(taskID, idx)); out != "" {
				return out
			}
		}
	}
	return BuildProxyURL(taskID, idx, defaultImageProxyTTL)
}
