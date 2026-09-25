<template>
  <div class="settlement-list">
    <el-page-header :content="'智能结算 - ' + (groupStore.current?.name || '')" @back="router.push(`/groups/${groupId}`)" />
    <el-card shadow="never" style="margin-top: 16px">
      <template #header>
        <div class="card-header">
          <span>结算建议（最小化转账次数，需付款方转账 + 收款方确认后才完成）</span>
          <el-button type="primary" :loading="generating" @click="handleGenerate">生成结算建议</el-button>
        </div>
      </template>
      <DataTable :data="settlementStore.settlements" :loading="loading">
        <el-table-column label="转账方向" min-width="180">
          <template #default="{ row }">
            <span class="flow">
              <span class="flow__name">{{ row.from_name || '用户#' + row.from_user_id }}</span>
              <el-icon color="#f56c6c"><Right /></el-icon>
              <span class="flow__name">{{ row.to_name || '用户#' + row.to_user_id }}</span>
            </span>
          </template>
        </el-table-column>
        <el-table-column label="金额" width="120">
          <template #default="{ row }"><MoneyText :value="row.amount" /></template>
        </el-table-column>
        <el-table-column label="状态" width="110">
          <template #default="{ row }"><StatusBadge :status="row.status" kind="settlement" /></template>
        </el-table-column>
        <el-table-column label="待操作" min-width="150">
          <template #default="{ row }">
            <span :class="{ 'actor--me': isMyTurn(row) }">{{ actorText(row) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="转账/确认时间" min-width="170">
          <template #default="{ row }">
            <div class="times">
              <span v-if="row.paid_at">转账：{{ row.paid_at }}</span>
              <span v-if="row.settled_at">确认：{{ row.settled_at }}</span>
              <span v-if="!row.paid_at && !row.settled_at">-</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="130" fixed="right">
          <template #default="{ row }">
            <el-button
              v-if="row.status === SettlementStatus.PENDING && row.from_user_id === myId"
              size="small"
              type="primary"
              text
              @click="handlePay(row)"
            >我已转账</el-button>
            <el-button
              v-else-if="row.status === SettlementStatus.PAID && row.to_user_id === myId"
              size="small"
              type="success"
              text
              @click="handleConfirm(row)"
            >确认收款</el-button>
            <span v-else class="op-hint">{{ waitingText(row) }}</span>
          </template>
        </el-table-column>
      </DataTable>
      <EmptyState v-if="!loading && settlementStore.settlements.length === 0" description="暂无结算建议，点击右上角生成">
        <el-button type="primary" size="small" @click="handleGenerate">立即生成</el-button>
      </EmptyState>
    </el-card>

    <ConfirmDialog
      ref="payDialog"
      title="我已转账"
      :message="`确认你已向 ${actionTarget?.to_name || '对方'} 转账 ¥${(actionTarget?.amount || 0).toFixed(2)} 吗？标记后需对方确认收款才完成。`"
      @confirm="doPay"
    />
    <ConfirmDialog
      ref="confirmDialog"
      title="确认收款"
      :message="`确认已收到 ${actionTarget?.from_name || '对方'} 转来的 ¥${(actionTarget?.amount || 0).toFixed(2)} 吗？确认后转账完成并更新双方净余额。`"
      @confirm="doConfirm"
    />

    <el-card shadow="never" style="margin-top: 16px">
      <template #header><span>成员净余额（仅双方确认的转账计入）</span></template>
      <el-table :data="settlementStore.balances" border stripe>
        <el-table-column prop="nickname" label="成员" min-width="140" />
        <el-table-column prop="username" label="用户名" min-width="120" />
        <el-table-column label="净余额" min-width="140">
          <template #default="{ row }">
            <MoneyText :value="row.net_amount" :tone="row.net_amount >= 0 ? 'income' : 'expense'" />
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Right } from '@element-plus/icons-vue'
import { useGroupStore } from '@/stores/group'
import { useSettlementStore } from '@/stores/settlement'
import { useAuthStore } from '@/stores/auth'
import { SettlementStatus } from '@/constants'
import type { SettlementInfo } from '@/api/settlement'
import DataTable from '@/components/DataTable.vue'
import EmptyState from '@/components/EmptyState.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import MoneyText from '@/components/MoneyText.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'

const route = useRoute()
const router = useRouter()
const groupStore = useGroupStore()
const settlementStore = useSettlementStore()
const authStore = useAuthStore()
const groupId = Number(route.params.id)

const myId = computed(() => authStore.user?.id || 0)
const loading = ref(false)
const generating = ref(false)

onMounted(async () => {
  loading.value = true
  try {
    if (!groupStore.current) {
      await groupStore.fetchGroup(groupId)
    }
    await settlementStore.fetchSettlements(groupId)
    await settlementStore.fetchBalances(groupId)
  } finally {
    loading.value = false
  }
})

// 是否轮到当前用户操作
function isMyTurn(row: SettlementInfo): boolean {
  return (
    (row.status === SettlementStatus.PENDING && row.from_user_id === myId.value) ||
    (row.status === SettlementStatus.PAID && row.to_user_id === myId.value)
  )
}

// “待操作”列：谁需要下一步操作
function actorText(row: SettlementInfo): string {
  const fromName = row.from_name || '用户#' + row.from_user_id
  const toName = row.to_name || '用户#' + row.to_user_id
  switch (row.status) {
    case SettlementStatus.PENDING:
      return `待 ${fromName} 转账`
    case SettlementStatus.PAID:
      return `待 ${toName} 确认收款`
    case SettlementStatus.SETTLED:
      return '双方已确认'
    default:
      return '-'
  }
}

// 非本人操作时的等待提示（不允许代确认）
function waitingText(row: SettlementInfo): string {
  const fromName = row.from_name || '对方'
  const toName = row.to_name || '对方'
  switch (row.status) {
    case SettlementStatus.PENDING:
      return `等待 ${fromName} 转账`
    case SettlementStatus.PAID:
      return `等待 ${toName} 确认`
    case SettlementStatus.SETTLED:
      return '已完成'
    default:
      return '-'
  }
}

async function refresh() {
  await settlementStore.fetchSettlements(groupId)
  await settlementStore.fetchBalances(groupId)
}

async function handleGenerate() {
  generating.value = true
  try {
    await settlementStore.generate(groupId)
    await settlementStore.fetchBalances(groupId)
    ElMessage.success('结算建议已生成')
  } finally {
    generating.value = false
  }
}

const payDialog = ref<InstanceType<typeof ConfirmDialog>>()
const confirmDialog = ref<InstanceType<typeof ConfirmDialog>>()
const actionTarget = ref<SettlementInfo>()

// 付款方点击"我已转账"：先二次确认，再标记（金额进入待确认，净余额暂不变化）
function handlePay(row: SettlementInfo) {
  actionTarget.value = row
  payDialog.value?.open()
}

// 收款方点击"确认收款"：先二次确认，再完成转账并更新双方净余额
function handleConfirm(row: SettlementInfo) {
  actionTarget.value = row
  confirmDialog.value?.open()
}

async function doPay() {
  if (!actionTarget.value) return
  await settlementStore.pay([actionTarget.value.id])
  await refresh()
  ElMessage.success('已标记转账，等待对方确认收款')
}

async function doConfirm() {
  if (!actionTarget.value) return
  await settlementStore.confirm([actionTarget.value.id])
  await refresh()
  ElMessage.success('收款已确认，转账完成')
}
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.flow {
  display: flex;
  align-items: center;
  gap: 8px;
}
.flow__name {
  font-weight: 600;
}
.actor--me {
  color: #e6a23c;
  font-weight: 600;
}
.times {
  display: flex;
  flex-direction: column;
  font-size: 12px;
  color: #909399;
}
.op-hint {
  color: #909399;
  font-size: 12px;
}
</style>
