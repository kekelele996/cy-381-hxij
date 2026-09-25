package dto

// SettlementResp 结算建议响应。
type SettlementResp struct {
	ID         uint    `json:"id"`
	GroupID    uint    `json:"group_id"`
	FromUserID uint    `json:"from_user_id"`
	FromName   string  `json:"from_name"`
	ToUserID   uint    `json:"to_user_id"`
	ToName     string  `json:"to_name"`
	Amount     float64 `json:"amount"`
	Status     string  `json:"status"`
	PaidAt     string  `json:"paid_at,omitempty"`
	SettledAt  string  `json:"settled_at,omitempty"`
	CreatedAt  string  `json:"created_at"`
}

// SettlementActionReq 结算状态流转请求（标记转账 / 确认收款共用）。
type SettlementActionReq struct {
	SettlementIDs []uint `json:"settlement_ids" binding:"required,min=1,dive,gt=0"`
}

// GroupBalance 成员结算余额（负数表示应付款）。
type GroupBalance struct {
	UserID    uint    `json:"user_id"`
	Username  string  `json:"username"`
	Nickname  string  `json:"nickname"`
	NetAmount float64 `json:"net_amount"`
}
