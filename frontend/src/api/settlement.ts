// 结算转账模块 API（双方确认：付款方「我已转账」→ 收款方「确认收款」）
import { get, post } from '@/utils/request'

export interface SettlementInfo {
  id: number
  group_id: number
  from_user_id: number
  from_name: string
  to_user_id: number
  to_name: string
  amount: number
  status: string
  awaiting_user_id: number // 当前需要操作的用户：pending=付款方，transferred=收款方，其余=0
  transferred_at?: string
  settled_at?: string
  created_at: string
}

export interface GroupBalance {
  user_id: number
  username: string
  nickname: string
  net_amount: number
}

export function generateSettlementsApi(groupId: number) {
  return post<{ list: SettlementInfo[]; total: number }>(`/groups/${groupId}/settlements/generate`)
}

export function listSettlementsApi(groupId: number) {
  return get<{ list: SettlementInfo[]; total: number }>(`/groups/${groupId}/settlements`)
}

export function listPendingSettlementsApi() {
  return get<{ list: SettlementInfo[]; total: number }>('/settlements/pending')
}

// 付款方确认「我已转账」，金额进入待确认，净余额暂不变化
export function markTransferredApi(settlementId: number) {
  return post<{ message: string; settlement: SettlementInfo }>('/settlements/transfer', {
    settlement_id: settlementId,
  })
}

// 收款方确认收款，转账完成并更新双方净余额
export function confirmReceivedApi(settlementId: number) {
  return post<{ message: string; settlement: SettlementInfo }>('/settlements/confirm', {
    settlement_id: settlementId,
  })
}

export function listBalancesApi(groupId: number) {
  return get<{ list: GroupBalance[]; total: number }>(`/groups/${groupId}/balances`)
}
