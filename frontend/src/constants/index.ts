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
  PENDING: 'pending',
  PAID: 'paid',
  SETTLED: 'settled',
  VOIDED: 'voided',
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
  { value: SettlementStatus.PENDING, label: '待转账' },
  { value: SettlementStatus.PAID, label: '待收款确认' },
  { value: SettlementStatus.SETTLED, label: '已完成' },
  { value: SettlementStatus.VOIDED, label: '已作废' },
]

export const GroupStatusOptions = [
  { value: GroupStatus.ACTIVE, label: '进行中' },
  { value: GroupStatus.ARCHIVED, label: '已归档' },
]
