package service

import (
	"testing"

	"github.com/aasplit/aasplit/internal/constants"
	"github.com/aasplit/aasplit/internal/dto"
	"github.com/aasplit/aasplit/internal/model"
	"github.com/aasplit/aasplit/internal/repository"
	"gorm.io/gorm"
)

// newSettlementSvc 基于测试数据库构造结算服务。
func newSettlementSvc(db *gorm.DB) *SettlementService {
	auditSvc := NewAuditService(repository.NewAuditRepository(db), newTestLogger())
	return NewSettlementService(db,
		repository.NewSettlementRepository(db),
		repository.NewExpenseShareRepository(db),
		repository.NewGroupMemberRepository(db),
		repository.NewGroupRepository(db),
		repository.NewUserRepository(db),
		auditSvc, newTestLogger())
}

// findTransfer 在结算建议中查找指定方向的转账。
func findTransfer(t *testing.T, items []model.Settlement, from, to uint) model.Settlement {
	t.Helper()
	for _, it := range items {
		if it.FromUserID == from && it.ToUserID == to {
			return it
		}
	}
	t.Fatalf("transfer %d -> %d not found", from, to)
	return model.Settlement{}
}

// balanceOf 查询指定成员的净余额。
func balanceOf(t *testing.T, svc *SettlementService, userID, groupID uint) float64 {
	t.Helper()
	balances, err := svc.Balances(userID, groupID)
	if err != nil {
		t.Fatalf("balances: %v", err)
	}
	for _, b := range balances {
		if b.UserID == userID {
			return b.NetAmount
		}
	}
	t.Fatalf("balance of user %d not found", userID)
	return 0
}

func TestSettlementServiceGenerate(t *testing.T) {
	db, expenseSvc, _, groupID, aliceID, bobID, carolID := newExpenseServiceFixture(t)

	// Alice 付 300 三人均摊 → Bob 应付 100，Carol 应付 100
	req := &dto.CreateExpenseReq{
		GroupID: groupID, Title: "火锅", Amount: 300, Category: "dining",
		PayerID: aliceID, SplitType: "equal", PaidAt: "2026-08-01 12:00:00",
		Shares: []dto.ShareInput{{UserID: aliceID}, {UserID: bobID}, {UserID: carolID}},
	}
	if _, err := expenseSvc.Create(aliceID, req); err != nil {
		t.Fatalf("create expense: %v", err)
	}

	svc := newSettlementSvc(db)
	items, err := svc.Generate(aliceID, groupID)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	total := 0.0
	for _, it := range items {
		total += it.Amount
		if it.Status != constants.SettlementPending {
			t.Fatalf("status = %s, want pending", it.Status)
		}
	}
	if len(items) != 2 {
		t.Fatalf("transfers = %d, want 2", len(items))
	}
	if total < 199.99 || total > 200.01 {
		t.Fatalf("transfer total = %.2f, want ~200", total)
	}
}

// TestSettlementTwoPhaseConfirm 双方确认流程：付款方标记转账 → 净余额不变 → 收款方确认 → 净余额更新。
func TestSettlementTwoPhaseConfirm(t *testing.T) {
	db, expenseSvc, _, groupID, aliceID, bobID, _ := newExpenseServiceFixture(t)
	if _, err := expenseSvc.Create(aliceID, &dto.CreateExpenseReq{
		GroupID: groupID, Title: "火锅", Amount: 300, Category: "dining",
		PayerID: aliceID, SplitType: "equal", PaidAt: "2026-08-01 12:00:00",
		Shares:  []dto.ShareInput{{UserID: aliceID}, {UserID: bobID}},
	}); err != nil {
		t.Fatalf("create expense: %v", err)
	}
	svc := newSettlementSvc(db)
	items, err := svc.Generate(aliceID, groupID)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	transfer := findTransfer(t, items, bobID, aliceID)

	// 收款方不能替付款方标记转账
	if _, err := svc.Pay(aliceID, &dto.SettlementActionReq{SettlementIDs: []uint{transfer.ID}}); err == nil {
		t.Fatal("pay by payee should fail")
	}
	// 付款方标记"我已转账"：状态进入待确认，净余额暂不变化
	if _, err := svc.Pay(bobID, &dto.SettlementActionReq{SettlementIDs: []uint{transfer.ID}}); err != nil {
		t.Fatalf("pay: %v", err)
	}
	if got := balanceOf(t, svc, aliceID, groupID); got != 150 {
		t.Fatalf("alice balance after pay = %.2f, want 150（确认前净余额不变）", got)
	}
	// 重复标记应失败（状态已非 pending）
	if _, err := svc.Pay(bobID, &dto.SettlementActionReq{SettlementIDs: []uint{transfer.ID}}); err == nil {
		t.Fatal("duplicate pay should fail")
	}
	// 付款方不能替收款方确认收款
	if _, err := svc.Confirm(bobID, &dto.SettlementActionReq{SettlementIDs: []uint{transfer.ID}}); err == nil {
		t.Fatal("confirm by payer should fail")
	}
	// 收款方确认收款：转账完成，双方净余额更新
	if _, err := svc.Confirm(aliceID, &dto.SettlementActionReq{SettlementIDs: []uint{transfer.ID}}); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if got := balanceOf(t, svc, aliceID, groupID); got != 0 {
		t.Fatalf("alice balance after confirm = %.2f, want 0", got)
	}
	if got := balanceOf(t, svc, bobID, groupID); got != 0 {
		t.Fatalf("bob balance after confirm = %.2f, want 0", got)
	}
	// 确认后不可再操作
	if _, err := svc.Confirm(aliceID, &dto.SettlementActionReq{SettlementIDs: []uint{transfer.ID}}); err == nil {
		t.Fatal("duplicate confirm should fail")
	}
	// 待处理列表应清空
	pending, err := svc.ListPending(bobID)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending after confirm = %d, want 0", len(pending))
	}
}

// TestSettlementVoidOnExpenseChange 账单变更后：未确认的转账作废并按新账重算，已确认收款的记录保留。
func TestSettlementVoidOnExpenseChange(t *testing.T) {
	db, expenseSvc, _, groupID, aliceID, bobID, _ := newExpenseServiceFixture(t)
	createReq := &dto.CreateExpenseReq{
		GroupID: groupID, Title: "火锅", Amount: 300, Category: "dining",
		PayerID: aliceID, SplitType: "equal", PaidAt: "2026-08-01 12:00:00",
		Shares:  []dto.ShareInput{{UserID: aliceID}, {UserID: bobID}},
	}
	created, err := expenseSvc.Create(aliceID, createReq)
	if err != nil {
		t.Fatalf("create expense: %v", err)
	}
	svc := newSettlementSvc(db)
	items, err := svc.Generate(aliceID, groupID)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	transfer := findTransfer(t, items, bobID, aliceID)

	// 第一笔：双方确认完成（应永久保留并抵减净余额）
	if _, err := svc.Pay(bobID, &dto.SettlementActionReq{SettlementIDs: []uint{transfer.ID}}); err != nil {
		t.Fatalf("pay: %v", err)
	}
	if _, err := svc.Confirm(aliceID, &dto.SettlementActionReq{SettlementIDs: []uint{transfer.ID}}); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	// 新增账单 → 触发重算，产生新的待转账建议
	if _, err := expenseSvc.Create(aliceID, &dto.CreateExpenseReq{
		GroupID: groupID, Title: "打车", Amount: 100, Category: "transport",
		PayerID: aliceID, SplitType: "equal", PaidAt: "2026-08-02 12:00:00",
		Shares:  []dto.ShareInput{{UserID: aliceID}, {UserID: bobID}},
	}); err != nil {
		t.Fatalf("create expense 2: %v", err)
	}
	items, err = svc.ListByGroup(aliceID, groupID)
	if err != nil {
		t.Fatalf("list by group: %v", err)
	}
	var settledCount, pendingCount int
	var newTransfer model.Settlement
	for _, it := range items {
		switch it.Status {
		case constants.SettlementSettled:
			settledCount++
			if it.ID != transfer.ID {
				t.Fatalf("settled record %d should be kept, got %d", transfer.ID, it.ID)
			}
		case constants.SettlementPending:
			pendingCount++
			newTransfer = it
		}
	}
	if settledCount != 1 {
		t.Fatalf("settled count = %d, want 1（已确认记录保留）", settledCount)
	}
	if pendingCount != 1 {
		t.Fatalf("pending count = %d, want 1（新账重算出一笔）", pendingCount)
	}
	if newTransfer.Amount != 50 {
		t.Fatalf("new transfer amount = %.2f, want 50", newTransfer.Amount)
	}

	// 新转账付款方标记后（未确认），修改账单 → 未确认转账作废并按新账重算
	if _, err := svc.Pay(bobID, &dto.SettlementActionReq{SettlementIDs: []uint{newTransfer.ID}}); err != nil {
		t.Fatalf("pay new transfer: %v", err)
	}
	if err := expenseSvc.Update(aliceID, created.ID, &dto.UpdateExpenseReq{
		Title: "火锅（改）", Amount: 100, Category: "dining",
		PayerID: aliceID, SplitType: "equal", PaidAt: "2026-08-01 12:00:00",
		Shares: []dto.ShareInput{{UserID: aliceID}, {UserID: bobID}},
	}); err != nil {
		t.Fatalf("update expense: %v", err)
	}
	items, err = svc.ListByGroup(aliceID, groupID)
	if err != nil {
		t.Fatalf("list by group: %v", err)
	}
	for _, it := range items {
		if it.ID == newTransfer.ID {
			t.Fatalf("unconfirmed transfer %d should be voided after expense update", newTransfer.ID)
		}
	}
	// 已确认记录仍保留
	if _, err := svc.Confirm(aliceID, &dto.SettlementActionReq{SettlementIDs: []uint{transfer.ID}}); err == nil {
		t.Fatal("confirmed record should not be actionable again")
	}
	// 净余额 = 新账（100+100 各半 → Bob 欠 100）- 已确认转账 150 → Alice -50（Bob 多付了 50）
	if got := balanceOf(t, svc, aliceID, groupID); got != -50 {
		t.Fatalf("alice balance = %.2f, want -50", got)
	}
	if got := balanceOf(t, svc, bobID, groupID); got != 50 {
		t.Fatalf("bob balance = %.2f, want 50", got)
	}

	// 退款后再次重算：未确认建议作废，净余额只保留已确认转账的影响
	if err := expenseSvc.Delete(aliceID, created.ID); err != nil {
		t.Fatalf("refund expense: %v", err)
	}
	if got := balanceOf(t, svc, aliceID, groupID); got != -100 {
		t.Fatalf("alice balance after refund = %.2f, want -100", got)
	}
}
