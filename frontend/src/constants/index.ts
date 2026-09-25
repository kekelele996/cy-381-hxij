// 与后端 internal/constants 对应的共享枚举（屎山耦合点 5）
export const UserRole = {
  USER: 'user',
  ADMIN: 'admin',
} as const

export const GroupStatus = {
  ACTIVE: 'active',
  ARCHIVED: 'archived',
} as const

export const ExpenseCategory = {
  DINING: 'dining',
  TRANSPORT: 'transport',
  LODGING: 'lodging',
  ENTERTAIN: 'entertain',
  OTHER: 'other',
} as const

export const SplitType = {
  EQUAL: 'equal',
  RATIO: 'ratio',
  AMOUNT: 'amount',
} as const

export const ExpenseStatus = {
  ACTIVE: 'active',
  REFUNDED: 'refunded',
} as const

export const SettlementStatus = {
  PENDING: 'pending', // 待付款：等待付款方点击「我已转账」
  TRANSFERRED: 'transferred', // 待收款确认：付款方已转账，等待收款方确认
  SETTLED: 'settled', // 已确认收款：双方净余额已更新
  VOIDED: 'voided', // 已作废：账单变更后未完成双方确认
} as const

export const ShareStatus = {
  UNSETTLED: 'unsettled',
  SETTLED: 'settled',
} as const

export const CategoryOptions = [
  { value: ExpenseCategory.DINING, label: '餐饮' },
  { value: ExpenseCategory.TRANSPORT, label: '交通' },
  { value: ExpenseCategory.LODGING, label: '住宿' },
  { value: ExpenseCategory.ENTERTAIN, label: '娱乐' },
  { value: ExpenseCategory.OTHER, label: '其他' },
]

export const SplitTypeOptions = [
  { value: SplitType.EQUAL, label: '均摊' },
  { value: SplitType.RATIO, label: '按比例' },
  { value: SplitType.AMOUNT, label: '按金额' },
]

export const SettlementStatusOptions = [
  { value: SettlementStatus.PENDING, label: '待付款' },
  { value: SettlementStatus.TRANSFERRED, label: '待收款确认' },
  { value: SettlementStatus.SETTLED, label: '已确认收款' },
  { value: SettlementStatus.VOIDED, label: '已作废' },
]

export const GroupStatusOptions = [
  { value: GroupStatus.ACTIVE, label: '进行中' },
  { value: GroupStatus.ARCHIVED, label: '已归档' },
]
