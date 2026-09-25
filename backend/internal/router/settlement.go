package router

import (
	"github.com/aasplit/aasplit/internal/handler"
	"github.com/aasplit/aasplit/internal/middleware"
	"github.com/aasplit/aasplit/internal/util"
	"github.com/gin-gonic/gin"
)

// RegisterSettlementRoutes 注册结算建议路由。
func RegisterSettlementRoutes(r *gin.RouterGroup, h *handler.SettlementHandler, jwt *util.JWTManager) {
	groups := r.Group("/groups/:id")
	groups.Use(middleware.Auth(jwt))
	{
		groups.POST("/settlements/generate", h.Generate)
		groups.GET("/settlements", h.ListByGroup)
		groups.GET("/balances", h.Balances)
	}
	settlements := r.Group("/settlements")
	settlements.Use(middleware.Auth(jwt))
	{
		settlements.GET("/pending", h.ListPending)
		// 双方确认：付款方先标记已转账，收款方再确认收款（服务端按 JWT 用户校验，禁止代确认）
		settlements.POST("/transfer", h.MarkTransferred)
		settlements.POST("/confirm", h.ConfirmReceived)
	}
}
