// 结算建议模块 API
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
  paid_at?: string
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

// 付款方标记"我已转账"（金额进入待确认，净余额暂不变化）
export function paySettlementApi(settlementIds: number[]) {
  return post<{ message: string; affected: number }>('/settlements/pay', { settlement_ids: settlementIds })
}

// 收款方确认收款（转账完成，更新双方净余额）
export function confirmSettlementApi(settlementIds: number[]) {
  return post<{ message: string; affected: number }>('/settlements/confirm', { settlement_ids: settlementIds })
}

export function listBalancesApi(groupId: number) {
  return get<{ list: GroupBalance[]; total: number }>(`/groups/${groupId}/balances`)
}
