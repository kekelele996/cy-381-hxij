<template>
  <div class="settlement-list">
    <el-page-header :content="'智能结算 - ' + (groupStore.current?.name || '')" @back="router.push(`/groups/${groupId}`)" />

    <el-alert
      class="rule-alert"
      type="info"
      :closable="false"
      show-icon
      title="双方确认规则"
      description="每笔转账需两步完成：①付款方点击「我已转账」，金额进入待确认，双方净余额暂时不动；②收款方核对是谁转来的钱后点击「确认收款」，转账才完成并更新净余额。账单新增、修改或退款后，尚未完成双方确认的转账会自动作废并按新账重算，已确认收款的记录保留。"
    />

    <el-card shadow="never" style="margin-top: 16px">
      <template #header>
        <div class="card-header">
          <span>结算转账（最小化转账次数）</span>
          <el-button type="primary" :loading="generating" @click="handleGenerate">按最新账单重算</el-button>
        </div>
      </template>

      <div class="todo-strip">
        <el-tag type="warning" effect="light">待我转账：{{ myToPayCount }} 笔</el-tag>
        <el-tag type="primary" effect="light">待我确认收款：{{ myToConfirmCount }} 笔</el-tag>
      </div>

      <DataTable :data="settlementStore.settlements" :loading="loading">
        <el-table-column label="转账方向" min-width="210">
          <template #default="{ row }">
            <span class="flow">
              <span :class="['flow__name', { 'flow__me': row.from_user_id === myId }]">
                {{ nameOf(row.from_user_id, row.from_name) }}<span v-if="row.from_user_id === myId" class="flow__me-tag">（我）</span>
              </span>
              <el-icon color="#f56c6c"><Right /></el-icon>
              <span :class="['flow__name', { 'flow__me': row.to_user_id === myId }]">
                {{ nameOf(row.to_user_id, row.to_name) }}<span v-if="row.to_user_id === myId" class="flow__me-tag">（我）</span>
              </span>
            </span>
          </template>
        </el-table-column>
        <el-table-column label="金额" width="120">
          <template #default="{ row }"><MoneyText :value="row.amount" /></template>
        </el-table-column>
        <el-table-column label="状态" width="115">
          <template #default="{ row }"><StatusBadge :status="row.status" kind="settlement" /></template>
        </el-table-column>
        <el-table-column label="待谁操作" min-width="150">
          <template #default="{ row }">
            <span v-if="row.status === SettlementStatus.PENDING" class="await">
              等待
              <strong :class="{ 'await__me': row.from_user_id === myId }">
                {{ row.from_user_id === myId ? '我（付款方）' : nameOf(row.from_user_id, row.from_name) + '（付款方）' }}
              </strong>
              转账
            </span>
            <span v-else-if="row.status === SettlementStatus.TRANSFERRED" class="await">
              等待
              <strong :class="{ 'await__me': row.to_user_id === myId }">
                {{ row.to_user_id === myId ? '我（收款方）' : nameOf(row.to_user_id, row.to_name) + '（收款方）' }}
              </strong>
              确认收款
            </span>
            <span v-else-if="row.status === SettlementStatus.SETTLED" class="muted">双方已完成</span>
            <span v-else class="muted">账单已变更</span>
          </template>
        </el-table-column>
        <el-table-column label="操作时间" min-width="160">
          <template #default="{ row }">
            <div v-if="row.transferred_at" class="time-line">转账：{{ row.transferred_at }}</div>
            <div v-if="row.settled_at" class="time-line">收款：{{ row.settled_at }}</div>
            <span v-if="!row.transferred_at && !row.settled_at" class="muted">-</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <!-- 仅付款方本人在待付款状态可见：收款方/其他人看不到，不能代操作 -->
            <el-button
              v-if="row.status === SettlementStatus.PENDING && row.from_user_id === myId"
              size="small"
              type="warning"
              text
              @click="handleMarkTransferred(row)"
            >
              我已转账
            </el-button>
            <!-- 仅收款方本人在待确认状态可见：付款方/其他人看不到，不能代确认 -->
            <el-button
              v-else-if="row.status === SettlementStatus.TRANSFERRED && row.to_user_id === myId"
              size="small"
              type="success"
              text
              @click="handleConfirm(row)"
            >
              确认收款
            </el-button>
            <span v-else-if="row.status === SettlementStatus.PENDING || row.status === SettlementStatus.TRANSFERRED" class="muted">
              {{ row.status === SettlementStatus.PENDING ? '需付款方操作' : '需收款方操作' }}
            </span>
            <span v-else class="muted">-</span>
          </template>
        </el-table-column>
      </DataTable>
      <EmptyState v-if="!loading && settlementStore.settlements.length === 0" description="暂无结算转账，新增账单后会自动生成，也可手动按最新账单重算">
        <el-button type="primary" size="small" @click="handleGenerate">立即重算</el-button>
      </EmptyState>
    </el-card>

    <el-card shadow="never" style="margin-top: 16px">
      <template #header>
        <div class="card-header">
          <span>成员净余额</span>
          <span class="muted small">仅已确认收款的转账计入；待确认金额不动</span>
        </div>
      </template>
      <el-table :data="settlementStore.balances" border stripe>
        <el-table-column label="成员" min-width="140">
          <template #default="{ row }">
            {{ row.nickname }}<span v-if="row.user_id === myId" class="flow__me-tag">（我）</span>
          </template>
        </el-table-column>
        <el-table-column prop="username" label="用户名" min-width="120" />
        <el-table-column label="剩余净余额（正=应收 / 负=应付）" min-width="220">
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
import { ElMessage, ElMessageBox } from 'element-plus'
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

const route = useRoute()
const router = useRouter()
const groupStore = useGroupStore()
const settlementStore = useSettlementStore()
const authStore = useAuthStore()
const groupId = Number(route.params.id)

const loading = ref(false)
const generating = ref(false)
const myId = computed(() => authStore.user?.id || 0)

const myToPayCount = computed(
  () => settlementStore.settlements.filter((s) => s.status === SettlementStatus.PENDING && s.from_user_id === myId.value).length,
)
const myToConfirmCount = computed(
  () => settlementStore.settlements.filter((s) => s.status === SettlementStatus.TRANSFERRED && s.to_user_id === myId.value).length,
)

function nameOf(userId: number, name: string) {
  return name || `用户#${userId}`
}

onMounted(async () => {
  loading.value = true
  try {
    if (!groupStore.current) {
      await groupStore.fetchGroup(groupId)
    }
    await refresh()
  } finally {
    loading.value = false
  }
})

async function refresh() {
  await Promise.all([
    settlementStore.fetchSettlements(groupId),
    settlementStore.fetchBalances(groupId),
    settlementStore.fetchPending(),
  ])
}

async function handleGenerate() {
  generating.value = true
  try {
    await settlementStore.generate(groupId)
    await refresh()
    ElMessage.success('已按最新账单重算，未确认的旧转账已作废')
  } finally {
    generating.value = false
  }
}

async function handleMarkTransferred(row: SettlementInfo) {
  try {
    await ElMessageBox.confirm(
      `确认你已向「${nameOf(row.to_user_id, row.to_name)}」转账 ¥${row.amount.toFixed(2)} 吗？提交后将等待对方确认收款，期间净余额不变。`,
      '我已转账',
      { confirmButtonText: '我已转账，提交', cancelButtonText: '取消', type: 'warning' },
    )
  } catch {
    return
  }
  await settlementStore.markTransferred(row.id)
  await refresh()
  ElMessage.success('已标记转账，等待收款方确认')
}

async function handleConfirm(row: SettlementInfo) {
  try {
    await ElMessageBox.confirm(
      `请确认已收到「${nameOf(row.from_user_id, row.from_name)}」转来的 ¥${row.amount.toFixed(2)}。确认后双方净余额将更新且不可撤销。`,
      '确认收款',
      { confirmButtonText: '已收到，确认', cancelButtonText: '取消', type: 'success' },
    )
  } catch {
    return
  }
  await settlementStore.confirmReceived(row.id)
  await refresh()
  ElMessage.success('收款确认成功，双方净余额已更新')
}
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.rule-alert {
  margin-top: 16px;
}
.todo-strip {
  display: flex;
  gap: 12px;
  margin-bottom: 12px;
}
.flow {
  display: flex;
  align-items: center;
  gap: 8px;
}
.flow__name {
  font-weight: 600;
}
.flow__me {
  color: #409eff;
}
.flow__me-tag {
  color: #409eff;
  font-size: 12px;
  font-weight: 600;
}
.await__me {
  color: #e6a23c;
}
.muted {
  color: #909399;
}
.small {
  font-size: 12px;
}
.time-line {
  font-size: 12px;
  color: #606266;
  line-height: 1.6;
}
</style>
