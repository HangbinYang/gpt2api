package image

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/432539/gpt2api/pkg/resp"
)

// AdminHandler 管理员视角下的生成记录接口。
type AdminHandler struct {
	dao      *DAO
	settings interface {
		SiteAPIBaseURL() string
	}
}

// NewAdminHandler 构造。
func NewAdminHandler(dao *DAO, settings interface{ SiteAPIBaseURL() string }) *AdminHandler {
	return &AdminHandler{dao: dao, settings: settings}
}

// List GET /api/admin/image-tasks
// 查询参数:page / page_size / user_id / keyword(prompt 或邮箱模糊) / status
func (h *AdminHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if size < 1 {
		size = 20
	}
	if size > 200 {
		size = 200
	}
	userID, _ := strconv.ParseUint(c.Query("user_id"), 10, 64)

	f := AdminTaskFilter{
		UserID:  userID,
		Keyword: strings.TrimSpace(c.Query("keyword")),
		Status:  strings.TrimSpace(c.Query("status")),
	}
	if raw := strings.TrimSpace(c.Query("start_at")); raw != "" {
		tm, ok := parseFilterTime(raw)
		if !ok {
			resp.Fail(c, resp.CodeBadRequest, "start_at 格式错误")
			return
		}
		f.Since = tm
	}
	if raw := strings.TrimSpace(c.Query("end_at")); raw != "" {
		tm, ok := parseFilterTime(raw)
		if !ok {
			resp.Fail(c, resp.CodeBadRequest, "end_at 格式错误")
			return
		}
		f.Until = tm.Add(time.Second)
	}

	rows, total, err := h.dao.ListAdmin(c.Request.Context(), f, size, (page-1)*size)
	if err != nil {
		resp.Internal(c, err.Error())
		return
	}

	type rowOut struct {
		AdminTaskRow
		ResultURLsParsed []string `json:"result_urls_parsed"`
	}
	baseURL := ""
	if h != nil && h.settings != nil {
		baseURL = h.settings.SiteAPIBaseURL()
	}
	out := make([]rowOut, 0, len(rows))
	for _, r := range rows {
		urls := BuildPublicImageURLs(baseURL, r.TaskID, r.DecodeFileIDs(), r.DecodeResultURLs())
		out = append(out, rowOut{
			AdminTaskRow:     r,
			ResultURLsParsed: urls,
		})
	}

	resp.OK(c, gin.H{
		"list":      out,
		"total":     total,
		"page":      page,
		"page_size": size,
	})
}
