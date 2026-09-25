// 结算转账状态管理（双方确认）
import { defineStore } from 'pinia'
import {
  confirmReceivedApi,
  generateSettlementsApi,
  listBalancesApi,
  listPendingSettlementsApi,
  listSettlementsApi,
  markTransferredApi,
  type GroupBalance,
  type SettlementInfo,
} from '@/api/settlement'

export const useSettlementStore = defineStore('settlement', {
  state: () => ({
    settlements: [] as SettlementInfo[],
    pending: [] as SettlementInfo[], // 等待当前用户操作的转账
    balances: [] as GroupBalance[],
  }),
  actions: {
    async fetchSettlements(groupId: number) {
      const data = await listSettlementsApi(groupId)
      this.settlements = data.list
    },
    async generate(groupId: number) {
      const data = await generateSettlementsApi(groupId)
      this.settlements = data.list
    },
    async fetchPending() {
      const data = await listPendingSettlementsApi()
      this.pending = data.list
    },
    // 付款方：我已转账
    async markTransferred(id: number) {
      await markTransferredApi(id)
      await this.fetchPending()
    },
    // 收款方：确认收款
    async confirmReceived(id: number) {
      await confirmReceivedApi(id)
      await this.fetchPending()
    },
    async fetchBalances(groupId: number) {
      const data = await listBalancesApi(groupId)
      this.balances = data.list
    },
  },
})
