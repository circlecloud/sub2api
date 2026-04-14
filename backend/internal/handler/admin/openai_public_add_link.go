package admin

import (
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type openAIPublicAddLinkRequest struct {
	Name            string                                      `json:"name"`
	GroupIDs        []int64                                     `json:"group_ids" binding:"required,min=1"`
	AccountDefaults *service.OpenAIPublicAddLinkAccountDefaults `json:"account_defaults,omitempty"`
}

type openAIPublicAddLinkResponse struct {
	Token           string                                      `json:"token"`
	Name            string                                      `json:"name"`
	GroupIDs        []int64                                     `json:"group_ids"`
	AccountDefaults *service.OpenAIPublicAddLinkAccountDefaults `json:"account_defaults,omitempty"`
	URL             string                                      `json:"url"`
	CreatedAt       time.Time                                   `json:"created_at"`
	UpdatedAt       time.Time                                   `json:"updated_at"`
}

func deriveOpenAIPublicAddLinkBaseURL(c *gin.Context) string {
	origin := strings.TrimSpace(c.GetHeader("Origin"))
	if origin != "" && strings.ToLower(origin) != "null" {
		return strings.TrimRight(origin, "/")
	}

	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if xfProto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); xfProto != "" {
		scheme = strings.TrimSpace(strings.Split(xfProto, ",")[0])
	}

	host := strings.TrimSpace(c.Request.Host)
	if xfHost := strings.TrimSpace(c.GetHeader("X-Forwarded-Host")); xfHost != "" {
		host = strings.TrimSpace(strings.Split(xfHost, ",")[0])
	}
	if host == "" {
		return ""
	}

	return scheme + "://" + host
}

func (h *OpenAIOAuthHandler) toOpenAIPublicAddLinkResponse(c *gin.Context, link *service.OpenAIPublicAddLink) openAIPublicAddLinkResponse {
	url := h.settingService.BuildOpenAIPublicAddLinkURL(c.Request.Context(), link.Token)
	if strings.HasPrefix(url, "/") {
		if baseURL := deriveOpenAIPublicAddLinkBaseURL(c); baseURL != "" {
			url = baseURL + url
		}
	}
	return openAIPublicAddLinkResponse{
		Token:           link.Token,
		Name:            link.Name,
		GroupIDs:        link.GroupIDs,
		AccountDefaults: link.AccountDefaults,
		URL:             url,
		CreatedAt:       link.CreatedAt,
		UpdatedAt:       link.UpdatedAt,
	}
}

func (h *OpenAIOAuthHandler) ListPublicAddLinks(c *gin.Context) {
	links, err := h.settingService.ListOpenAIPublicAddLinks(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items := make([]openAIPublicAddLinkResponse, 0, len(links))
	for i := range links {
		items = append(items, h.toOpenAIPublicAddLinkResponse(c, &links[i]))
	}
	response.Success(c, items)
}

func (h *OpenAIOAuthHandler) CreatePublicAddLink(c *gin.Context) {
	var req openAIPublicAddLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	groupIDs, err := h.validateOpenAIPublicLinkGroupIDs(c.Request.Context(), req.GroupIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	accountDefaults, err := h.validateOpenAIPublicLinkAccountDefaults(c.Request.Context(), req.AccountDefaults)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	link, err := h.settingService.CreateOpenAIPublicAddLink(c.Request.Context(), req.Name, groupIDs, accountDefaults)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, h.toOpenAIPublicAddLinkResponse(c, link))
}

func (h *OpenAIOAuthHandler) UpdatePublicAddLink(c *gin.Context) {
	var req openAIPublicAddLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	groupIDs, err := h.validateOpenAIPublicLinkGroupIDs(c.Request.Context(), req.GroupIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	accountDefaults, err := h.validateOpenAIPublicLinkAccountDefaults(c.Request.Context(), req.AccountDefaults)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	link, err := h.settingService.UpdateOpenAIPublicAddLink(c.Request.Context(), c.Param("token"), req.Name, groupIDs, accountDefaults)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, h.toOpenAIPublicAddLinkResponse(c, link))
}

func (h *OpenAIOAuthHandler) RotatePublicAddLink(c *gin.Context) {
	link, err := h.settingService.RotateOpenAIPublicAddLink(c.Request.Context(), c.Param("token"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, h.toOpenAIPublicAddLinkResponse(c, link))
}

func (h *OpenAIOAuthHandler) DeletePublicAddLink(c *gin.Context) {
	if err := h.settingService.DeleteOpenAIPublicAddLink(c.Request.Context(), c.Param("token")); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "OpenAI public add link deleted successfully"})
}

func (h *OpenAIOAuthHandler) GetPublicAddLinkGroups(c *gin.Context) {
	link, err := h.requireOpenAIPublicAddLink(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	groups, err := h.listAllowedOpenAIGroupDTOs(c.Request.Context(), link.GroupIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, groups)
}

func (h *OpenAIOAuthHandler) GeneratePublicAddLinkAuthURL(c *gin.Context) {
	link, err := h.requireOpenAIPublicAddLink(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	defaults, err := h.resolveOpenAIPublicLinkAccountDefaults(c.Request.Context(), link)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	result, err := h.openaiOAuthService.GenerateAuthURL(c.Request.Context(), defaults.ProxyID, "", service.PlatformOpenAI)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
