package repository

import (
	"errors"
	"fmt"
	"time"

	"github.com/aasplit/aasplit/internal/constants"
	"github.com/aasplit/aasplit/internal/model"
	"gorm.io/gorm"
)

// ErrSettlementNotFound 结算建议不存在哨兵错误。
var ErrSettlementNotFound = errors.New("settlement not found")

// SettlementRepository 结算建议数据访问。
type SettlementRepository struct {
	db *gorm.DB
}

// NewSettlementRepository 构造结算建议仓储。
func NewSettlementRepository(db *gorm.DB) *SettlementRepository {
	return &SettlementRepository{db: db}
}

// CreateBatch 批量创建结算建议（事务中调用）。
func (r *SettlementRepository) CreateBatch(tx *gorm.DB, items []model.Settlement) error {
	if len(items) == 0 {
		return nil
	}
	if err := tx.Create(&items).Error; err != nil {
		return fmt.Errorf("create settlements: %w", err)
	}
	return nil
}

// ListByGroup 查询群组结算建议（含双方用户信息；已作废记录不参与展示）。
func (r *SettlementRepository) ListByGroup(groupID uint) ([]model.Settlement, error) {
	var items []model.Settlement
	if err := r.db.Preload("FromUser").Preload("ToUser").
		Where("group_id = ? AND status <> ?", groupID, constants.SettlementVoided).
		Order("status ASC, id ASC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list settlements by group: %w", err)
	}
	return items, nil
}

// ListPendingByUser 查询用户待处理的结算建议（待转账或待确认收款，含等待对方操作的项）。
func (r *SettlementRepository) ListPendingByUser(userID uint) ([]model.Settlement, error) {
	var items []model.Settlement
	if err := r.db.Preload("FromUser").Preload("ToUser").
		Where("(from_user_id = ? OR to_user_id = ?) AND status IN ?", userID, userID,
			[]string{string(constants.SettlementPending), string(constants.SettlementPaid)}).
		Order("id DESC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list pending settlements: %w", err)
	}
	return items, nil
}

// FindByID 按 ID 查询。
func (r *SettlementRepository) FindByID(id uint) (*model.Settlement, error) {
	var s model.Settlement
	if err := r.db.First(&s, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSettlementNotFound
		}
		return nil, fmt.Errorf("find settlement by id: %w", err)
	}
	return &s, nil
}

// MarkPaid 付款方标记"我已转账"：pending → paid，仅允许付款方本人操作。
func (r *SettlementRepository) MarkPaid(tx *gorm.DB, ids []uint, payerID uint) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	now := time.Now()
	res := tx.Model(&model.Settlement{}).
		Where("id IN ? AND status = ? AND from_user_id = ?", ids, constants.SettlementPending, payerID).
		Updates(map[string]interface{}{"status": constants.SettlementPaid, "paid_at": now})
	if res.Error != nil {
		return 0, fmt.Errorf("mark settlements paid: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// MarkConfirmed 收款方确认收款：paid → settled，仅允许收款方本人操作。
func (r *SettlementRepository) MarkConfirmed(tx *gorm.DB, ids []uint, payeeID uint) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	now := time.Now()
	res := tx.Model(&model.Settlement{}).
		Where("id IN ? AND status = ? AND to_user_id = ?", ids, constants.SettlementPaid, payeeID).
		Updates(map[string]interface{}{"status": constants.SettlementSettled, "settled_at": now})
	if res.Error != nil {
		return 0, fmt.Errorf("mark settlements confirmed: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// VoidUnconfirmedByGroup 作废群组内尚未双方确认的结算建议（pending/paid → voided），已确认收款的记录保留。
func (r *SettlementRepository) VoidUnconfirmedByGroup(tx *gorm.DB, groupID uint) (int64, error) {
	res := tx.Model(&model.Settlement{}).
		Where("group_id = ? AND status IN ?", groupID,
			[]string{string(constants.SettlementPending), string(constants.SettlementPaid)}).
		Update("status", constants.SettlementVoided)
	if res.Error != nil {
		return 0, fmt.Errorf("void unconfirmed settlements: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// SumSettledByGroup 统计群组内已确认收款的转账总额（sent=转出，received=转入），用于净余额调整。
// 传入事务句柄可读到未提交变更，传入普通 db 则按已提交数据读取。
func (r *SettlementRepository) SumSettledByGroup(db *gorm.DB, groupID uint) (sent, received map[uint]float64, err error) {
	type row struct {
		FromUserID uint
		ToUserID   uint
		Amount     float64
	}
	var rows []row
	if err := db.Model(&model.Settlement{}).
		Select("from_user_id, to_user_id, amount").
		Where("group_id = ? AND status = ?", groupID, constants.SettlementSettled).
		Scan(&rows).Error; err != nil {
		return nil, nil, fmt.Errorf("sum settled settlements: %w", err)
	}
	sent = make(map[uint]float64, len(rows))
	received = make(map[uint]float64, len(rows))
	for _, rw := range rows {
		sent[rw.FromUserID] += rw.Amount
		received[rw.ToUserID] += rw.Amount
	}
	return sent, received, nil
}
