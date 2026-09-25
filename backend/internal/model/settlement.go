package model

import (
	"time"

	"github.com/aasplit/aasplit/internal/constants"
)

// Settlement 结算建议实体：智能结算路径中的一笔转账，需付款方与收款方双方确认。
// 状态机：pending（待付款）→ transferred（待收款确认）→ settled（已确认收款）；
// 账单新增/修改/退款后，尚未双方确认（pending/transferred）的记录作废为 voided，已 settled 的保留。
type Settlement struct {
	ID            uint                       `gorm:"primaryKey" json:"id"`
	GroupID       uint                       `gorm:"index;not null" json:"group_id"`
	FromUserID    uint                       `gorm:"index;not null" json:"from_user_id"`
	ToUserID      uint                       `gorm:"index;not null" json:"to_user_id"`
	Amount        float64                    `gorm:"type:double precision;not null" json:"amount"`
	Status        constants.SettlementStatus `gorm:"size:16;not null;default:pending;index" json:"status"`
	TransferredAt *time.Time                 `json:"transferred_at,omitempty"` // 付款方点击「我已转账」的时间
	SettledAt     *time.Time                 `json:"settled_at,omitempty"`     // 收款方确认收款的时间
	CreatedAt     time.Time                  `json:"created_at"`
	UpdatedAt     time.Time                  `json:"updated_at"`

	FromUser *User `gorm:"foreignKey:FromUserID" json:"from_user,omitempty"`
	ToUser   *User `gorm:"foreignKey:ToUserID" json:"to_user,omitempty"`
}

// TableName 指定表名。
func (Settlement) TableName() string { return "settlements" }

// IsPending 判断是否待付款（等待付款方转账）。
func (s *Settlement) IsPending() bool { return s.Status == constants.SettlementPending }

// IsTransferred 判断是否已转账、等待收款方确认。
func (s *Settlement) IsTransferred() bool { return s.Status == constants.SettlementTransferred }

// IsSettled 判断收款方是否已确认收款。
func (s *Settlement) IsSettled() bool { return s.Status == constants.SettlementSettled }

// IsVoid 判断是否因账单变更而作废。
func (s *Settlement) IsVoid() bool { return s.Status == constants.SettlementVoided }

// AwaitingAction 返回当前需要操作的一方用户 ID：pending 为付款方，transferred 为收款方，否则为 0。
func (s *Settlement) AwaitingAction() uint {
	switch s.Status {
	case constants.SettlementPending:
		return s.FromUserID
	case constants.SettlementTransferred:
		return s.ToUserID
	default:
		return 0
	}
}
