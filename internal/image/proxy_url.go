package image

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// imageProxySecret 进程级随机密钥,用于 HMAC 签名图片 URL。
// 进程重启后旧的签名 URL 全部失效,这是故意的(防止长期有效的 URL 泄漏)。
var imageProxySecret []byte

func init() {
	imageProxySecret = make([]byte, 32)
	if _, err := rand.Read(imageProxySecret); err != nil {
		for i := range imageProxySecret {
			imageProxySecret[i] = byte(i*31 + 7)
		}
	}
}

// BuildProxyURL 生成代理 URL。返回绝对 path(不含 host)。
func BuildProxyURL(taskID string, idx int, ttl time.Duration) string {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	expMs := time.Now().Add(ttl).UnixMilli()
	sig := computeImgSig(taskID, idx, expMs)
	return fmt.Sprintf("/p/img/%s/%d?exp=%d&sig=%s", taskID, idx, expMs, sig)
}

// BuildPublicProxyURL 在相对代理路径基础上按需补全 host。
// apiBaseURL 允许填写完整的 /v1 地址,这里只提取 scheme://host[:port]。
// 若 apiBaseURL 为空/非法,或 path 已经是绝对 URL,则保持原值不变。
func BuildPublicProxyURL(apiBaseURL, taskID string, idx int, ttl time.Duration) string {
	return WithPublicBaseURL(BuildProxyURL(taskID, idx, ttl), apiBaseURL)
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
