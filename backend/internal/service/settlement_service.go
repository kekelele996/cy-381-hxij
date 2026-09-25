package service

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/aasplit/aasplit/internal/constants"
	"github.com/aasplit/aasplit/internal/dto"
	"github.com/aasplit/aasplit/internal/model"
	"github.com/aasplit/aasplit/internal/repository"
	"github.com/aasplit/aasplit/internal/util"
	"github.com/aasplit/aasplit/pkg/splitcalc"
	"gorm.io/gorm"
)

// SettlementService 结算建议业务逻辑：付款方/收款方双方确认状态机。
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

// Generate 重新生成智能结算建议（事务 + 群组行锁）。
// 保留已确认收款（settled）的记录；作废 pending/transferred 的旧建议并按最新账单重算剩余转账。
func (s *SettlementService) Generate(userID, groupID uint) ([]model.Settlement, error) {
	var result []model.Settlement
	err := s.db.Transaction(func(tx *gorm.DB) error {
		group, err := s.groupRepo.LockByID(groupID)
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
		items, _, err := s.rebuildInTx(tx, groupID, memberIDs)
		if err != nil {
			return err
		}
		result = items
		return nil
	})
	if err != nil {
		return nil, err
	}
	// 重新查询以加载 FromUser/ToUser 信息
	result, err = s.settleRepo.ListByGroup(groupID)
	if err != nil {
		return nil, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogSettlementGenerated, groupID, len(result), userID))
	s.auditSvc.Record(userID, string(constants.ActionSettlementGenerate), "group", fmt.Sprint(groupID), "生成智能结算建议", "")
	return result, nil
}

// RebuildOnExpenseChange 账单新增/修改/退款后调用：尚未双方确认的转账作废并按新账重算，已确认收款的保留。
// 必须在调用方事务内、已持有群组行锁时调用；operatorID/expenseID 仅用于日志。
// 返回作废条数与新生成条数；均为 0 表示该群组没有结算建议被影响，调用方可跳过日志。
func (s *SettlementService) RebuildOnExpenseChange(tx *gorm.DB, groupID, operatorID, expenseID uint) (voided, created int, err error) {
	memberIDs, err := s.memberRepo.ListUserIDs(groupID)
	if err != nil {
		return 0, 0, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	items, voidedCount, err := s.rebuildInTx(tx, groupID, memberIDs)
	if err != nil {
		return 0, 0, err
	}
	if voidedCount > 0 || len(items) > 0 {
		s.logger.Info(fmt.Sprintf(constants.LogSettlementVoidRebuild, groupID, voidedCount, len(items), operatorID, expenseID))
	}
	return int(voidedCount), len(items), nil
}

// rebuildInTx 在事务内执行「作废未确认 → 扣除已确认收款 → 重算剩余转账」。调用方必须持有群组行锁。
func (s *SettlementService) rebuildInTx(tx *gorm.DB, groupID uint, memberIDs []uint) ([]model.Settlement, int64, error) {
	// 1. 作废所有尚未双方确认（pending/transferred）的转账；settled 记录保留。
	voided, err := s.settleRepo.VoidUnsettledByGroup(tx, groupID)
	if err != nil {
		return nil, 0, err
	}

	// 2. 计算剩余净余额 = 账单原始净余额 − 已确认收款的转账净额；待确认中的转账不影响余额。
	paid, err := s.shareRepo.SumPaidByGroupOn(tx, groupID)
	if err != nil {
		return nil, 0, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	owed, err := s.shareRepo.SumOwedByGroupOn(tx, groupID)
	if err != nil {
		return nil, 0, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	settledNet, err := s.settleRepo.SumSettledByGroupOn(tx, groupID)
	if err != nil {
		return nil, 0, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	balances := make([]splitcalc.Balance, 0, len(memberIDs))
	for _, mid := range memberIDs {
		residual := util.Round2(paid[mid] - owed[mid] - settledNet[mid])
		balances = append(balances, splitcalc.Balance{UserID: mid, Amount: residual})
	}

	// 3. 按剩余净余额生成新的待付款转账。
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
		return nil, 0, err
	}
	return items, voided, nil
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

// ListPending 查询等待当前用户操作的转账（我是付款方且待转账，或我是收款方且待确认）。
func (s *SettlementService) ListPending(userID uint) ([]model.Settlement, error) {
	items, err := s.settleRepo.ListPendingByUser(userID)
	if err != nil {
		return nil, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	return items, nil
}

// MarkTransferred 付款方点击「我已转账」：仅付款方本人可操作，金额进入待确认，净余额不变（事务 + 行锁）。
func (s *SettlementService) MarkTransferred(userID uint, req *dto.SettlementActionReq) (*model.Settlement, error) {
	var item *model.Settlement
	err := s.db.Transaction(func(tx *gorm.DB) error {
		settle, err := s.settleRepo.FindByIDForUpdate(tx, req.SettlementID)
		if err != nil {
			return util.Wrap(constants.CodeNotFound, "结算转账 settlement 不存在", err)
		}
		// 只有付款方本人能标记已转账，收款方不能代操作。
		if settle.FromUserID != userID {
			return util.NewAppError(constants.CodeSettlementWrongParty,
				fmt.Sprintf("结算转账 settlement_id=%d 的付款方为 user_id=%d，当前用户 user_id=%d 无权代其确认转账", settle.ID, settle.FromUserID, userID), nil)
		}
		if settle.Status != constants.SettlementPending {
			return util.NewAppError(constants.CodeSettlementWrongState,
				fmt.Sprintf("结算转账 settlement_id=%d 当前状态为 %s，无法标记已转账（仅待付款 pending 可操作）", settle.ID, settle.Status), nil)
		}
		affected, err := s.settleRepo.MarkTransferred(tx, settle.ID, time.Now())
		if err != nil {
			return util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
		}
		if affected == 0 {
			return util.NewAppError(constants.CodeSettlementWrongState, constants.MsgErrSettlementState, nil)
		}
		item = settle
		return nil
	})
	if err != nil {
		return nil, err
	}
	detail, err := s.settleRepo.FindDetailByID(item.ID)
	if err != nil {
		return nil, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogSettlementTransferred, detail.ID, detail.FromUserID, detail.ToUserID, detail.Amount, userID))
	s.auditSvc.Record(userID, string(constants.ActionSettlementTransfer), "settlement", fmt.Sprint(detail.ID),
		fmt.Sprintf("付款方标记已转账 %.2f，等待收款方确认", detail.Amount), "")
	return detail, nil
}

// ConfirmReceived 收款方确认收款：仅收款方本人可操作，确认后转账完成并更新双方净余额（事务 + 行锁）。
func (s *SettlementService) ConfirmReceived(userID uint, req *dto.SettlementActionReq) (*model.Settlement, error) {
	var item *model.Settlement
	err := s.db.Transaction(func(tx *gorm.DB) error {
		settle, err := s.settleRepo.FindByIDForUpdate(tx, req.SettlementID)
		if err != nil {
			return util.Wrap(constants.CodeNotFound, "结算转账 settlement 不存在", err)
		}
		// 只有收款方本人能确认收款，付款方不能代确认。
		if settle.ToUserID != userID {
			return util.NewAppError(constants.CodeSettlementWrongParty,
				fmt.Sprintf("结算转账 settlement_id=%d 的收款方为 user_id=%d，当前用户 user_id=%d 无权代其确认收款", settle.ID, settle.ToUserID, userID), nil)
		}
		if settle.Status != constants.SettlementTransferred {
			return util.NewAppError(constants.CodeSettlementWrongState,
				fmt.Sprintf("结算转账 settlement_id=%d 当前状态为 %s，无法确认收款（仅待收款确认 transferred 可操作）", settle.ID, settle.Status), nil)
		}
		affected, err := s.settleRepo.MarkSettled(tx, settle.ID, time.Now())
		if err != nil {
			return util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
		}
		if affected == 0 {
			return util.NewAppError(constants.CodeSettlementWrongState, constants.MsgErrSettlementState, nil)
		}
		item = settle
		return nil
	})
	if err != nil {
		return nil, err
	}
	detail, err := s.settleRepo.FindDetailByID(item.ID)
	if err != nil {
		return nil, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogSettlementConfirmed, detail.ID, detail.FromUserID, detail.ToUserID, detail.Amount, userID))
	s.auditSvc.Record(userID, string(constants.ActionSettlementConfirm), "settlement", fmt.Sprint(detail.ID),
		fmt.Sprintf("收款方确认收款 %.2f，双方净余额已更新", detail.Amount), "")
	return detail, nil
}

// Balances 计算群组成员净余额（统计页/结算页共用）：账单净余额扣除已确认收款的转账；待确认金额不动。
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
	settledNet, err := s.settleRepo.SumSettledByGroup(groupID)
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
			NetAmount: util.Round2(paid[m.UserID] - owed[m.UserID] - settledNet[m.UserID]),
		})
	}
	return balances, nil
}
