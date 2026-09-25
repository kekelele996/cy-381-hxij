package model

import (
	"time"

	"github.com/aasplit/aasplit/internal/constants"
)

// Settlement 结算建议实体：智能结算路径中的一笔转账。
// 状态机：pending（待转账）→ paid（付款方已标记，待收款确认）→ settled（收款方已确认，完成）；
// 账单变更时 pending/paid 被作废为 voided 并按新账重算，settled 记录永久保留。
type Settlement struct {
	ID         uint                       `gorm:"primaryKey" json:"id"`
	GroupID    uint                       `gorm:"index;not null" json:"group_id"`
	FromUserID uint                       `gorm:"index;not null" json:"from_user_id"`
	ToUserID   uint                       `gorm:"index;not null" json:"to_user_id"`
	Amount     float64                    `gorm:"type:double precision;not null" json:"amount"`
	Status     constants.SettlementStatus `gorm:"size:16;not null;default:pending;index" json:"status"`
	PaidAt     *time.Time                 `json:"paid_at,omitempty"`
	SettledAt  *time.Time                 `json:"settled_at,omitempty"`
	CreatedAt  time.Time                  `json:"created_at"`
	UpdatedAt  time.Time                  `json:"updated_at"`

	FromUser *User `gorm:"foreignKey:FromUserID" json:"from_user,omitempty"`
	ToUser   *User `gorm:"foreignKey:ToUserID" json:"to_user,omitempty"`
}

// TableName 指定表名。
func (Settlement) TableName() string { return "settlements" }

// IsPending 判断是否待转账。
func (s *Settlement) IsPending() bool { return s.Status == constants.SettlementPending }

// IsPaid 判断付款方是否已标记转账（待收款方确认）。
func (s *Settlement) IsPaid() bool { return s.Status == constants.SettlementPaid }

// IsSettled 判断是否双方确认完成。
func (s *Settlement) IsSettled() bool { return s.Status == constants.SettlementSettled }
