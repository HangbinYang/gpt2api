package image

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/432539/gpt2api/internal/middleware"
	"github.com/432539/gpt2api/pkg/resp"
)

// MeHandler 面向当前用户的图片任务只读接口(JWT 鉴权)。
// 与 /v1/images/tasks/:id(API Key 鉴权)共享同一张 image_tasks 表,
// 只是入口改到 /api/me/images/* 便于前端面板调用。
type MeHandler struct {
	dao      *DAO
	settings interface {
		SiteAPIBaseURL() string
	}
}

// NewMeHandler 构造。
func NewMeHandler(dao *DAO, settings interface{ SiteAPIBaseURL() string }) *MeHandler {
	return &MeHandler{dao: dao, settings: settings}
}

// taskView 是对外返回的视图结构,解码 JSON 列 + 隐藏内部字段。
type taskView struct {
	ID             uint64     `json:"id"`
	TaskID         string     `json:"task_id"`
	UserID         uint64     `json:"user_id"`
	ModelID        uint64     `json:"model_id"`
	AccountID      uint64     `json:"account_id"`
	Prompt         string     `json:"prompt"`
	N              int        `json:"n"`
	Size           string     `json:"size"`
	Upscale        string     `json:"upscale,omitempty"`
	Status         string     `json:"status"`
	ConversationID string     `json:"conversation_id,omitempty"`
	Error          string     `json:"error,omitempty"`
	CreditCost     int64      `json:"credit_cost"`
	ImageURLs      []string   `json:"image_urls"`
	FileIDs        []string   `json:"file_ids,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
}

func (h *MeHandler) toView(t *Task) taskView {
	fileIDs := t.DecodeFileIDs()
	for i, id := range fileIDs {
		fileIDs[i] = strings.TrimPrefix(id, "sed:")
	}

	baseURL := ""
	if h != nil && h.settings != nil {
		baseURL = h.settings.SiteAPIBaseURL()
	}
	urls := BuildPublicImageURLs(baseURL, t.TaskID, fileIDs, t.DecodeResultURLs())

	return taskView{
		ID:             t.ID,
		TaskID:         t.TaskID,
		UserID:         t.UserID,
		ModelID:        t.ModelID,
		AccountID:      t.AccountID,
		Prompt:         t.Prompt,
		N:              t.N,
		Size:           t.Size,
		Upscale:        t.Upscale,
		Status:         t.Status,
		ConversationID: t.ConversationID,
		Error:          t.Error,
		CreditCost:     t.CreditCost,
		ImageURLs:      urls,
		FileIDs:        fileIDs,
		CreatedAt:      t.CreatedAt,
		StartedAt:      t.StartedAt,
		FinishedAt:     t.FinishedAt,
	}
}

// GET /api/me/images/tasks
// 查询参数:
//
//	limit(默认 20,上限 100), offset
//	status            = queued | dispatched | running | success | failed
//	keyword           = prompt 模糊匹配
//	start_at, end_at  = 时间区间;支持 RFC3339、"2006-01-02 15:04:05"、"2006-01-02"
func (h *MeHandler) List(c *gin.Context) {
	uid := middleware.UserID(c)
	if uid == 0 {
		resp.Unauthorized(c, "not logged in")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	filter := UserTaskFilter{
		Status:  strings.TrimSpace(c.Query("status")),
		Keyword: strings.TrimSpace(c.Query("keyword")),
	}
	if raw := strings.TrimSpace(c.Query("start_at")); raw != "" {
		tm, ok := parseFilterTime(raw)
		if !ok {
			resp.Fail(c, resp.CodeBadRequest, "start_at 格式错误")
			return
		}
		filter.Since = tm
	}
	if raw := strings.TrimSpace(c.Query("end_at")); raw != "" {
		tm, ok := parseFilterTime(raw)
		if !ok {
			resp.Fail(c, resp.CodeBadRequest, "end_at 格式错误")
			return
		}
		filter.Until = tm.Add(time.Second)
	}

	tasks, total, err := h.dao.ListByUserFiltered(c.Request.Context(), uid, filter, limit, offset)
	if err != nil {
		resp.Internal(c, err.Error())
		return
	}
	items := make([]taskView, 0, len(tasks))
	for i := range tasks {
		items = append(items, h.toView(&tasks[i]))
	}
	resp.OK(c, gin.H{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// parseFilterTime 兼容前端常见的几种时间字面量写法,所有字符串都按
// 服务器本地时区解析,匹配 image_tasks.created_at 的 DATETIME 语义。
func parseFilterTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// GET /api/me/images/tasks/:id
func (h *MeHandler) Get(c *gin.Context) {
	uid := middleware.UserID(c)
	if uid == 0 {
		resp.Unauthorized(c, "not logged in")
		return
	}
	id := c.Param("id")
	if id == "" {
		resp.Fail(c, 40000, "task id required")
		return
	}
	t, err := h.dao.Get(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			resp.Fail(c, 40400, "task not found")
			return
		}
		resp.Internal(c, err.Error())
		return
	}
	if t.UserID != uid {
		resp.Fail(c, 40400, "task not found")
		return
	}
	resp.OK(c, h.toView(t))
}
