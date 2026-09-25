package dto

// SettlementResp 结算建议响应（一笔需双方确认的转账）。
type SettlementResp struct {
	ID             uint    `json:"id"`
	GroupID        uint    `json:"group_id"`
	FromUserID     uint    `json:"from_user_id"`
	FromName       string  `json:"from_name"`
	ToUserID       uint    `json:"to_user_id"`
	ToName         string  `json:"to_name"`
	Amount         float64 `json:"amount"`
	Status         string  `json:"status"`
	AwaitingUserID uint    `json:"awaiting_user_id"` // 当前需要操作的用户：pending=付款方，transferred=收款方，其余=0
	TransferredAt  string  `json:"transferred_at,omitempty"`
	SettledAt      string  `json:"settled_at,omitempty"`
	CreatedAt      string  `json:"created_at"`
}

// SettlementActionReq 单笔转账操作请求（我已转账 / 确认收款）。
type SettlementActionReq struct {
	SettlementID uint `json:"settlement_id" binding:"required,gt=0"`
}

// GroupBalance 成员结算余额（负数表示应付款；已确认收款的转账已计入）。
type GroupBalance struct {
	UserID    uint    `json:"user_id"`
	Username  string  `json:"username"`
	Nickname  string  `json:"nickname"`
	NetAmount float64 `json:"net_amount"`
}
