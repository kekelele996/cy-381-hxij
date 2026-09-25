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

// ListByGroup 查询群组全部结算建议（含双方用户信息；未完成在前，作废/完成在后）。
func (r *SettlementRepository) ListByGroup(groupID uint) ([]model.Settlement, error) {
	var items []model.Settlement
	if err := r.db.Preload("FromUser").Preload("ToUser").
		Where("group_id = ?", groupID).
		Order("CASE status WHEN 'pending' THEN 0 WHEN 'transferred' THEN 1 WHEN 'settled' THEN 2 ELSE 3 END, id ASC").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list settlements by group: %w", err)
	}
	return items, nil
}

// ListPendingByUser 查询等待当前用户操作的转账（待我转账 / 待我确认收款）。
func (r *SettlementRepository) ListPendingByUser(userID uint) ([]model.Settlement, error) {
	var items []model.Settlement
	if err := r.db.Preload("FromUser").Preload("ToUser").
		Where("(from_user_id = ? AND status = ?) OR (to_user_id = ? AND status = ?)",
			userID, string(constants.SettlementPending),
			userID, string(constants.SettlementTransferred)).
		Order("status DESC, id DESC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list pending settlements: %w", err)
	}
	return items, nil
}

// ListByGroupAndStatus 查询群组指定状态的结算建议。
func (r *SettlementRepository) ListByGroupAndStatus(tx *gorm.DB, groupID uint, statuses []constants.SettlementStatus) ([]model.Settlement, error) {
	var items []model.Settlement
	if err := tx.Where("group_id = ? AND status IN ?", groupID, statuses).
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list settlements by group and status: %w", err)
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

// FindDetailByID 按 ID 查询并预载付款方/收款方信息。
func (r *SettlementRepository) FindDetailByID(id uint) (*model.Settlement, error) {
	var s model.Settlement
	if err := r.db.Preload("FromUser").Preload("ToUser").First(&s, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSettlementNotFound
		}
		return nil, fmt.Errorf("find settlement detail by id: %w", err)
	}
	return &s, nil
}

// FindByIDForUpdate 按 ID 行级锁查询（事务中调用，SELECT ... FOR UPDATE）。
func (r *SettlementRepository) FindByIDForUpdate(tx *gorm.DB, id uint) (*model.Settlement, error) {
	var s model.Settlement
	if err := tx.Clauses(lockClause).First(&s, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSettlementNotFound
		}
		return nil, fmt.Errorf("find settlement by id for update: %w", err)
	}
	return &s, nil
}

// MarkTransferred 付款方确认已转账：仅 pending → transferred（条件更新，防并发）。
func (r *SettlementRepository) MarkTransferred(tx *gorm.DB, id uint, now time.Time) (int64, error) {
	res := tx.Model(&model.Settlement{}).
		Where("id = ? AND status = ?", id, string(constants.SettlementPending)).
		Updates(map[string]interface{}{"status": string(constants.SettlementTransferred), "transferred_at": now})
	if res.Error != nil {
		return 0, fmt.Errorf("mark settlement transferred: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// MarkSettled 收款方确认收款：仅 transferred → settled（条件更新，防并发/防代确认）。
func (r *SettlementRepository) MarkSettled(tx *gorm.DB, id uint, now time.Time) (int64, error) {
	res := tx.Model(&model.Settlement{}).
		Where("id = ? AND status = ?", id, string(constants.SettlementTransferred)).
		Updates(map[string]interface{}{"status": string(constants.SettlementSettled), "settled_at": now})
	if res.Error != nil {
		return 0, fmt.Errorf("mark settlement settled: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// VoidUnsettledByGroup 账单变更后作废群组内尚未双方确认（pending/transferred）的转账（事务中调用）。
func (r *SettlementRepository) VoidUnsettledByGroup(tx *gorm.DB, groupID uint) (int64, error) {
	res := tx.Model(&model.Settlement{}).
		Where("group_id = ? AND status IN ?", groupID, []constants.SettlementStatus{constants.SettlementPending, constants.SettlementTransferred}).
		Update("status", string(constants.SettlementVoided))
	if res.Error != nil {
		return 0, fmt.Errorf("void unsettled settlements: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// SumSettledByGroup 统计群组成员已确认收款的转账净额（from→to 方向，仅 settled）。
// 返回每个用户的净收入：收款为正、付款为负；只有双方确认完成的转账才计入。
func (r *SettlementRepository) SumSettledByGroup(groupID uint) (map[uint]float64, error) {
	return r.SumSettledByGroupOn(r.db, groupID)
}

// SumSettledByGroupOn 在指定事务/连接上统计群组成员已确认收款的转账净额。
func (r *SettlementRepository) SumSettledByGroupOn(tx *gorm.DB, groupID uint) (map[uint]float64, error) {
	type row struct {
		UserID uint
		Amount float64
	}
	var rows []row
	if err := tx.Raw(`SELECT user_id, SUM(delta) AS amount FROM (
		SELECT to_user_id AS user_id, amount AS delta FROM settlements WHERE group_id = ? AND status = ?
		UNION ALL
		SELECT from_user_id AS user_id, -amount AS delta FROM settlements WHERE group_id = ? AND status = ?
	) AS settled_flows GROUP BY user_id`,
		groupID, string(constants.SettlementSettled),
		groupID, string(constants.SettlementSettled)).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("sum settled settlements by group: %w", err)
	}
	result := make(map[uint]float64, len(rows))
	for _, rw := range rows {
		result[rw.UserID] = rw.Amount
	}
	return result, nil
}
