package service

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/aasplit/aasplit/internal/constants"
	"github.com/aasplit/aasplit/internal/dto"
	"github.com/aasplit/aasplit/internal/model"
	"github.com/aasplit/aasplit/internal/repository"
	"github.com/aasplit/aasplit/internal/util"
	"github.com/aasplit/aasplit/pkg/splitcalc"
	"gorm.io/gorm"
)

// SettlementService 结算建议业务逻辑。
type SettlementService struct {
	db         *gorm.DB
	settleRepo *repository.SettlementRepository
	shareRepo  *repository.ExpenseShareRepository
	memberRepo *repository.GroupMemberRepository
	groupRepo  *repository.GroupRepository
	userRepo   *repository.UserRepository
	auditSvc   *AuditService
	logger     *slog.Logger
}

// NewSettlementService 构造结算服务。
func NewSettlementService(db *gorm.DB, settleRepo *repository.SettlementRepository, shareRepo *repository.ExpenseShareRepository, memberRepo *repository.GroupMemberRepository, groupRepo *repository.GroupRepository, userRepo *repository.UserRepository, auditSvc *AuditService, logger *slog.Logger) *SettlementService {
	return &SettlementService{db: db, settleRepo: settleRepo, shareRepo: shareRepo, memberRepo: memberRepo, groupRepo: groupRepo, userRepo: userRepo, auditSvc: auditSvc, logger: logger}
}

// Generate 生成智能结算建议（事务 + 群组行锁 + 作废未确认建议 + 保留已确认记录）。
func (s *SettlementService) Generate(userID, groupID uint) ([]model.Settlement, error) {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		group, err := s.groupRepo.LockByIDTx(tx, groupID)
		if err != nil {
			return util.Wrap(constants.CodeNotFound, "群组 group 不存在", err)
		}
		if !group.IsActive() {
			return util.NewAppError(constants.CodeConflict, "群组 group 已归档，无法生成结算建议 settlement", nil)
		}
		memberIDs, err := s.memberRepo.ListUserIDs(groupID)
		if err != nil {
			return util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
		}
		if len(memberIDs) < 2 {
			return util.NewAppError(constants.CodeBadRequest, "群组成员 member 不足 2 人，无法生成结算建议 settlement", nil)
		}
		_, err = s.recalculateTx(tx, groupID, memberIDs)
		return err
	})
	if err != nil {
		return nil, err
	}
	// 重新查询以加载 FromUser/ToUser 信息
	result, err := s.settleRepo.ListByGroup(groupID)
	if err != nil {
		return nil, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogSettlementGenerated, groupID, len(result), userID))
	s.auditSvc.Record(userID, string(constants.ActionSettlementGenerate), "group", fmt.Sprint(groupID), "生成智能结算建议", "")
	return result, nil
}

// RecalculateTx 账单变更后重算群组结算建议（在调用方事务内执行）：
// 作废尚未双方确认的转账，按新净余额生成新建议；已确认收款的记录保留。
func (s *SettlementService) RecalculateTx(tx *gorm.DB, groupID, operatorID uint) error {
	memberIDs, err := s.memberRepo.ListUserIDs(groupID)
	if err != nil {
		return util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	if len(memberIDs) < 2 {
		// 成员不足时仅作废未确认建议，不再生成新建议
		if _, err := s.settleRepo.VoidUnconfirmedByGroup(tx, groupID); err != nil {
			return util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
		}
		return nil
	}
	voided, err := s.recalculateTx(tx, groupID, memberIDs)
	if err != nil {
		return err
	}
	if voided > 0 {
		s.logger.Info(fmt.Sprintf(constants.LogSettlementVoided, groupID, voided, operatorID))
	}
	return nil
}

// recalculateTx 作废未确认建议并按当前净余额（含已确认转账）生成新建议，返回作废数量。
func (s *SettlementService) recalculateTx(tx *gorm.DB, groupID uint, memberIDs []uint) (int64, error) {
	voided, err := s.settleRepo.VoidUnconfirmedByGroup(tx, groupID)
	if err != nil {
		return 0, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	paid, err := s.shareRepo.SumPaidByGroupTx(tx, groupID)
	if err != nil {
		return 0, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	owed, err := s.shareRepo.SumOwedByGroupTx(tx, groupID)
	if err != nil {
		return 0, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	sent, received, err := s.settleRepo.SumSettledByGroup(tx, groupID)
	if err != nil {
		return 0, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	balances := make([]splitcalc.Balance, 0, len(memberIDs))
	for _, mid := range memberIDs {
		balances = append(balances, splitcalc.Balance{UserID: mid, Amount: netAmount(paid, owed, sent, received, mid)})
	}
	transfers := splitcalc.OptimizeTransfers(balances)
	items := make([]model.Settlement, 0, len(transfers))
	for _, tr := range transfers {
		items = append(items, model.Settlement{
			GroupID:    groupID,
			FromUserID: tr.FromUserID,
			ToUserID:   tr.ToUserID,
			Amount:     tr.Amount,
			Status:     constants.SettlementPending,
		})
	}
	if err := s.settleRepo.CreateBatch(tx, items); err != nil {
		return 0, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	return voided, nil
}

// ListByGroup 查询群组结算建议。
func (s *SettlementService) ListByGroup(userID, groupID uint) ([]model.Settlement, error) {
	ok, err := s.memberRepo.Exists(groupID, userID)
	if err != nil {
		return nil, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	if !ok {
		return nil, util.NewAppError(constants.CodeNotGroupMember, constants.MsgErrNotGroupMember, nil)
	}
	items, err := s.settleRepo.ListByGroup(groupID)
	if err != nil {
		return nil, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	return items, nil
}

// ListPending 查询我的待处理结算提醒（待我转账/待我确认/等待对方）。
func (s *SettlementService) ListPending(userID uint) ([]model.Settlement, error) {
	items, err := s.settleRepo.ListPendingByUser(userID)
	if err != nil {
		return nil, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	return items, nil
}

// Pay 付款方标记"我已转账"（pending → paid；仅付款方本人可操作，净余额暂不变化）。
func (s *SettlementService) Pay(userID uint, req *dto.SettlementActionReq) (int64, error) {
	first, err := s.checkActionable(req.SettlementIDs, func(item *model.Settlement) error {
		if item.FromUserID != userID {
			return util.NewAppError(constants.CodeForbidden, constants.MsgErrSettlementNotPayer, nil)
		}
		if !item.IsPending() {
			return util.NewAppError(constants.CodeConflict, constants.MsgErrSettlementStatus, nil)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	var affected int64
	err = s.db.Transaction(func(tx *gorm.DB) error {
		n, err := s.settleRepo.MarkPaid(tx, req.SettlementIDs, userID)
		if err != nil {
			return util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
		}
		affected = n
		return nil
	})
	if err != nil {
		return 0, err
	}
	if first != nil {
		s.logger.Info(fmt.Sprintf(constants.LogSettlementPaid, first.ID, first.FromUserID, first.ToUserID, first.Amount, userID))
	}
	s.auditSvc.Record(userID, string(constants.ActionSettlementPay), "settlement", joinIDs(req.SettlementIDs), "付款方标记已转账，等待收款方确认", "")
	return affected, nil
}

// Confirm 收款方确认收款（paid → settled；仅收款方本人可操作，确认后更新双方净余额）。
func (s *SettlementService) Confirm(userID uint, req *dto.SettlementActionReq) (int64, error) {
	first, err := s.checkActionable(req.SettlementIDs, func(item *model.Settlement) error {
		if item.ToUserID != userID {
			return util.NewAppError(constants.CodeForbidden, constants.MsgErrSettlementNotPayee, nil)
		}
		if !item.IsPaid() {
			return util.NewAppError(constants.CodeConflict, constants.MsgErrSettlementStatus, nil)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	var affected int64
	err = s.db.Transaction(func(tx *gorm.DB) error {
		n, err := s.settleRepo.MarkConfirmed(tx, req.SettlementIDs, userID)
		if err != nil {
			return util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
		}
		affected = n
		return nil
	})
	if err != nil {
		return 0, err
	}
	if first != nil {
		s.logger.Info(fmt.Sprintf(constants.LogSettlementConfirmed, first.ID, first.FromUserID, first.ToUserID, first.Amount, userID))
	}
	s.auditSvc.Record(userID, string(constants.ActionSettlementConfirm), "settlement", joinIDs(req.SettlementIDs), "收款方确认收款，转账完成", "")
	return affected, nil
}

// checkActionable 逐笔加载并校验结算建议是否允许当前操作，返回第一笔用于日志。
func (s *SettlementService) checkActionable(ids []uint, check func(item *model.Settlement) error) (*model.Settlement, error) {
	var first *model.Settlement
	for _, id := range ids {
		item, err := s.settleRepo.FindByID(id)
		if err != nil {
			return nil, util.Wrap(constants.CodeNotFound, "结算建议 settlement 不存在", err)
		}
		if err := check(item); err != nil {
			return nil, err
		}
		if first == nil {
			first = item
		}
	}
	return first, nil
}

// Balances 计算群组成员净余额（统计页/结算页共用；含已确认收款转账的抵减）。
func (s *SettlementService) Balances(userID, groupID uint) ([]dto.GroupBalance, error) {
	ok, err := s.memberRepo.Exists(groupID, userID)
	if err != nil {
		return nil, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	if !ok {
		return nil, util.NewAppError(constants.CodeNotGroupMember, constants.MsgErrNotGroupMember, nil)
	}
	members, err := s.memberRepo.ListByGroup(groupID)
	if err != nil {
		return nil, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	paid, err := s.shareRepo.SumPaidByGroup(groupID)
	if err != nil {
		return nil, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	owed, err := s.shareRepo.SumOwedByGroup(groupID)
	if err != nil {
		return nil, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	sent, received, err := s.settleRepo.SumSettledByGroup(s.db, groupID)
	if err != nil {
		return nil, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	balances := make([]dto.GroupBalance, 0, len(members))
	for _, m := range members {
		if m.User == nil {
			continue
		}
		balances = append(balances, dto.GroupBalance{
			UserID:    m.UserID,
			Username:  m.User.Username,
			Nickname:  m.User.Nickname,
			NetAmount: netAmount(paid, owed, sent, received, m.UserID),
		})
	}
	return balances, nil
}

// netAmount 计算成员净余额：付款 - 应付 + 已确认转出 - 已确认转入（正数表示应收回）。
func netAmount(paid, owed, sent, received map[uint]float64, userID uint) float64 {
	return util.Round2(paid[userID] - owed[userID] + sent[userID] - received[userID])
}

// joinIDs 拼接结算 ID 列表用于审计资源标识。
func joinIDs(ids []uint) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprint(id))
	}
	return strings.Join(parts, ",")
}
