package service

import (
	"testing"

	"github.com/aasplit/aasplit/internal/constants"
	"github.com/aasplit/aasplit/internal/dto"
	"github.com/aasplit/aasplit/internal/repository"
	"github.com/aasplit/aasplit/internal/util"
)

// newSettlementServiceFixture 复用账单夹具构造结算服务。
func buildSettlementFixture(t *testing.T) (db interface{}, svc *SettlementService, expenseSvc *ExpenseService, groupID, aliceID, bobID, carolID uint) {
	t.Helper()
	gdb, eSvc, _, gid, aID, bID, cID := newExpenseServiceFixture(t)
	settleRepo := repository.NewSettlementRepository(gdb)
	shareRepo := repository.NewExpenseShareRepository(gdb)
	memberRepo := repository.NewGroupMemberRepository(gdb)
	groupRepo := repository.NewGroupRepository(gdb)
	userRepo := repository.NewUserRepository(gdb)
	auditSvc := NewAuditService(repository.NewAuditRepository(gdb), newTestLogger())
	sSvc := NewSettlementService(gdb, settleRepo, shareRepo, memberRepo, groupRepo, userRepo, auditSvc, newTestLogger())
	return gdb, sSvc, eSvc, gid, aID, bID, cID
}

func createHotpotExpense(t *testing.T, svc *ExpenseService, groupID, aliceID, bobID, carolID uint) {
	t.Helper()
	req := &dto.CreateExpenseReq{
		GroupID: groupID, Title: "火锅", Amount: 300, Category: "dining",
		PayerID: aliceID, SplitType: "equal", PaidAt: "2026-08-01 12:00:00",
		Shares: []dto.ShareInput{{UserID: aliceID}, {UserID: bobID}, {UserID: carolID}},
	}
	if _, err := svc.Create(aliceID, req); err != nil {
		t.Fatalf("create expense: %v", err)
	}
}

// TestSettlementTwoPartyConfirmFlow 付款方转账 → 收款方确认的完整双方确认流程。
func TestSettlementTwoPartyConfirmFlow(t *testing.T) {
	_, svc, expenseSvc, groupID, aliceID, bobID, carolID := buildSettlementFixture(t)
	createHotpotExpense(t, expenseSvc, groupID, aliceID, bobID, carolID)

	// 账单新增即按新账自动生成待付款转账（Bob→Alice 100、Carol→Alice 100）
	items, err := svc.ListByGroup(aliceID, groupID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("transfers = %d, want 2", len(items))
	}
	for _, it := range items {
		if it.Status != constants.SettlementPending {
			t.Fatalf("initial status = %s, want pending", it.Status)
		}
	}

	// 找到 Bob → Alice 的那笔（100）
	var bobTransfer uint
	for _, it := range items {
		if it.FromUserID == bobID {
			bobTransfer = it.ID
		}
	}
	if bobTransfer == 0 {
		t.Fatalf("bob transfer not found")
	}

	// 转账前：Bob 净余额 -100（待确认不影响余额，此时本来也没转账）
	bals, err := svc.Balances(aliceID, groupID)
	if err != nil {
		t.Fatalf("balances: %v", err)
	}
	bobNet := 0.0
	for _, b := range bals {
		if b.UserID == bobID {
			bobNet = b.NetAmount
		}
	}
	if bobNet > -99.99 || bobNet < -100.01 {
		t.Fatalf("bob net before transfer = %.2f, want -100", bobNet)
	}

	// 收款方 Alice 不能替 Bob 点「我已转账」
	if _, err := svc.MarkTransferred(aliceID, &dto.SettlementActionReq{SettlementID: bobTransfer}); err == nil {
		t.Fatalf("payee must not mark transferred for payer")
	} else if ae := util.AsAppError(err); ae == nil || ae.Code != constants.CodeSettlementWrongParty {
		t.Fatalf("err = %v, want CodeSettlementWrongParty", err)
	}

	// 付款方 Bob 标记已转账 → transferred，净余额不变
	if _, err := svc.MarkTransferred(bobID, &dto.SettlementActionReq{SettlementID: bobTransfer}); err != nil {
		t.Fatalf("mark transferred: %v", err)
	}
	bals, _ = svc.Balances(aliceID, groupID)
	for _, b := range bals {
		if b.UserID == bobID && (b.NetAmount < -100.01 || b.NetAmount > -99.99) {
			t.Fatalf("bob net after transfer = %.2f, want still -100 (pending confirm)", b.NetAmount)
		}
	}

	// 待确认列表：Bob（付款方）不应再看到这笔，Alice（收款方）应看到
	bobPending, err := svc.ListPending(bobID)
	if err != nil {
		t.Fatalf("list bob pending: %v", err)
	}
	for _, p := range bobPending {
		if p.ID == bobTransfer {
			t.Fatalf("bob should not await action on transferred item")
		}
	}
	alicePending, err := svc.ListPending(aliceID)
	if err != nil {
		t.Fatalf("list alice pending: %v", err)
	}
	found := false
	for _, p := range alicePending {
		if p.ID == bobTransfer {
			found = true
		}
	}
	if !found {
		t.Fatalf("alice should see transferred item awaiting confirm")
	}

	// 付款方 Bob 不能替 Alice 确认收款
	if _, err := svc.ConfirmReceived(bobID, &dto.SettlementActionReq{SettlementID: bobTransfer}); err == nil {
		t.Fatalf("payer must not confirm receipt for payee")
	} else if ae := util.AsAppError(err); ae == nil || ae.Code != constants.CodeSettlementWrongParty {
		t.Fatalf("err = %v, want CodeSettlementWrongParty", err)
	}

	// 收款方 Alice 确认收款 → settled，双方净余额更新
	if _, err := svc.ConfirmReceived(aliceID, &dto.SettlementActionReq{SettlementID: bobTransfer}); err != nil {
		t.Fatalf("confirm received: %v", err)
	}
	bals, _ = svc.Balances(aliceID, groupID)
	netByUser := map[uint]float64{}
	for _, b := range bals {
		netByUser[b.UserID] = b.NetAmount
	}
	if netByUser[bobID] < -0.01 || netByUser[bobID] > 0.01 {
		t.Fatalf("bob net after confirm = %.2f, want 0", netByUser[bobID])
	}
	// Alice 原始 +200，收到 Bob 的 100 → 剩余 +100
	if netByUser[aliceID] < 99.99 || netByUser[aliceID] > 100.01 {
		t.Fatalf("alice net after confirm = %.2f, want 100", netByUser[aliceID])
	}
	if netByUser[carolID] > -99.99 || netByUser[carolID] < -100.01 {
		t.Fatalf("carol net = %.2f, want -100", netByUser[carolID])
	}

	// 已 settled 的记录不能重复确认
	if _, err := svc.ConfirmReceived(aliceID, &dto.SettlementActionReq{SettlementID: bobTransfer}); err == nil {
		t.Fatalf("settled item must not be confirmable again")
	} else if ae := util.AsAppError(err); ae == nil || ae.Code != constants.CodeSettlementWrongState {
		t.Fatalf("err = %v, want CodeSettlementWrongState", err)
	}
}

// TestSettlementVoidOnExpenseChange 账单新增/修改/退款后未确认转账作废重算，已确认保留。
func TestSettlementVoidOnExpenseChange(t *testing.T) {
	_, svc, expenseSvc, groupID, aliceID, bobID, carolID := buildSettlementFixture(t)
	createHotpotExpense(t, expenseSvc, groupID, aliceID, bobID, carolID)

	items, err := svc.ListByGroup(aliceID, groupID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var bobTransfer, carolTransfer uint
	for _, it := range items {
		switch it.FromUserID {
		case bobID:
			bobTransfer = it.ID
		case carolID:
			carolTransfer = it.ID
		}
	}

	// Bob 完成双方确认
	if _, err := svc.MarkTransferred(bobID, &dto.SettlementActionReq{SettlementID: bobTransfer}); err != nil {
		t.Fatalf("bob transfer: %v", err)
	}
	if _, err := svc.ConfirmReceived(aliceID, &dto.SettlementActionReq{SettlementID: bobTransfer}); err != nil {
		t.Fatalf("alice confirm: %v", err)
	}

	// Carol 仅转账、尚未被确认
	if _, err := svc.MarkTransferred(carolID, &dto.SettlementActionReq{SettlementID: carolTransfer}); err != nil {
		t.Fatalf("carol transfer: %v", err)
	}

	// 新增一笔账单 → 触发作废重算
	req2 := &dto.CreateExpenseReq{
		GroupID: groupID, Title: "打车", Amount: 90, Category: "transport",
		PayerID: bobID, SplitType: "equal", PaidAt: "2026-08-02 12:00:00",
		Shares: []dto.ShareInput{{UserID: aliceID}, {UserID: bobID}, {UserID: carolID}},
	}
	if _, err := expenseSvc.Create(aliceID, req2); err != nil {
		t.Fatalf("create second expense: %v", err)
	}

	latest, err := svc.ListByGroup(aliceID, groupID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var settledKept, carolVoided bool
	pendingTotal := 0.0
	for _, it := range latest {
		switch it.ID {
		case bobTransfer:
			if it.Status != constants.SettlementSettled {
				t.Fatalf("confirmed transfer status = %s, want settled kept", it.Status)
			}
			settledKept = true
		case carolTransfer:
			if it.Status != constants.SettlementVoided {
				t.Fatalf("unconfirmed transferred item status = %s, want voided", it.Status)
			}
			carolVoided = true
		default:
			if it.Status != constants.SettlementPending {
				t.Fatalf("new transfer status = %s, want pending", it.Status)
			}
			pendingTotal += it.Amount
		}
	}
	if !settledKept || !carolVoided {
		t.Fatalf("kept=%v voided=%v", settledKept, carolVoided)
	}

	// 重算后剩余净余额：原始（Alice +200, Bob -100, Carol -100）+ 打车（Bob +60, Alice/Carol -30）
	// → Alice +170, Bob -40, Carol -130；扣除已确认 Bob→Alice 100
	// → 剩余 Alice +70, Bob +60, Carol -130，新转账应为 Carol→Alice 70、Carol→Bob 60，合计 130
	if pendingTotal < 129.99 || pendingTotal > 130.01 {
		t.Fatalf("new pending total = %.2f, want 130", pendingTotal)
	}
	bals, _ := svc.Balances(aliceID, groupID)
	for _, b := range bals {
		switch b.UserID {
		case aliceID:
			if b.NetAmount < 69.99 || b.NetAmount > 70.01 {
				t.Fatalf("alice residual = %.2f, want 70", b.NetAmount)
			}
		case bobID:
			if b.NetAmount < 59.99 || b.NetAmount > 60.01 {
				t.Fatalf("bob residual = %.2f, want 60", b.NetAmount)
			}
		case carolID:
			if b.NetAmount > -129.99 || b.NetAmount < -130.01 {
				t.Fatalf("carol residual = %.2f, want -130", b.NetAmount)
			}
		}
	}

	// 作废的转账不能再被确认
	if _, err := svc.ConfirmReceived(aliceID, &dto.SettlementActionReq{SettlementID: carolTransfer}); err == nil {
		t.Fatalf("voided item must not be confirmable")
	}
}

// TestSettlementVoidOnExpenseRefund 退款后待确认转账作废、重算为零转账；修改账单同样触发重算。
func TestSettlementVoidOnExpenseRefund(t *testing.T) {
	_, svc, expenseSvc, groupID, aliceID, bobID, carolID := buildSettlementFixture(t)
	created, err := expenseSvc.Create(aliceID, &dto.CreateExpenseReq{
		GroupID: groupID, Title: "火锅", Amount: 300, Category: "dining",
		PayerID: aliceID, SplitType: "equal", PaidAt: "2026-08-01 12:00:00",
		Shares: []dto.ShareInput{{UserID: aliceID}, {UserID: bobID}, {UserID: carolID}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	items, _ := svc.ListByGroup(aliceID, groupID)
	if len(items) != 2 {
		t.Fatalf("initial transfers = %d, want 2", len(items))
	}
	// 付款方刚标记转账、尚未被收款方确认
	bobItem := items[0]
	if items[0].FromUserID != bobID {
		bobItem = items[1]
	}
	if _, err := svc.MarkTransferred(bobItem.FromUserID, &dto.SettlementActionReq{SettlementID: bobItem.ID}); err != nil {
		t.Fatalf("mark transferred: %v", err)
	}

	// 退款该账单
	if err := expenseSvc.Delete(aliceID, created.ID); err != nil {
		t.Fatalf("refund: %v", err)
	}
	latest, err := svc.ListByGroup(aliceID, groupID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(latest) != 2 {
		t.Fatalf("rows after refund = %d, want 2 voided rows kept", len(latest))
	}
	for _, it := range latest {
		if it.Status != constants.SettlementVoided {
			t.Fatalf("status after refund = %s, want voided", it.Status)
		}
	}
	bals, _ := svc.Balances(aliceID, groupID)
	for _, b := range bals {
		if b.NetAmount < -0.01 || b.NetAmount > 0.01 {
			t.Fatalf("net after refund for user %d = %.2f, want 0", b.UserID, b.NetAmount)
		}
	}

	// 重新记账（成员仍为三人）→ 自动生成新的待付款转账
	if _, err := expenseSvc.Create(aliceID, &dto.CreateExpenseReq{
		GroupID: groupID, Title: "新火锅", Amount: 150, Category: "dining",
		PayerID: carolID, SplitType: "equal", PaidAt: "2026-08-05 12:00:00",
		Shares: []dto.ShareInput{{UserID: aliceID}, {UserID: bobID}, {UserID: carolID}},
	}); err != nil {
		t.Fatalf("re-create: %v", err)
	}
	latest, _ = svc.ListByGroup(aliceID, groupID)
	pending := 0
	pendingTotal := 0.0
	for _, it := range latest {
		if it.Status == constants.SettlementPending {
			pending++
			pendingTotal += it.Amount
		}
	}
	if pending != 2 {
		t.Fatalf("new pending transfers = %d, want 2", pending)
	}
	// Carol 付 150 三人均摊 → Alice、Bob 各付 Carol 50，合计 100
	if pendingTotal < 99.99 || pendingTotal > 100.01 {
		t.Fatalf("new pending total = %.2f, want 100", pendingTotal)
	}
}
