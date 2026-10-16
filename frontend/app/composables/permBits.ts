import { Perm, hasPerm } from '~/composables/useApi'

export const PERM_FLAGS = [
  { bit: Perm.Read, label: 'Read', short: 'R' },
  { bit: Perm.Create, label: 'Create', short: 'C' },
  { bit: Perm.Update, label: 'Update', short: 'U' },
  { bit: Perm.Delete, label: 'Delete', short: 'D' },
  { bit: Perm.UpdatePermissions, label: 'ACL', short: 'A' },
  { bit: Perm.Share, label: 'Share', short: 'S' },
] as const

export function cycleAllowDeny(allow: number, deny: number, bit: number) {
  const allowed = hasPerm(allow, bit)
  const denied = hasPerm(deny, bit)
  if (!allowed && !denied) {
    return { allow: allow | bit, deny }
  }
  if (allowed) {
    return { allow: allow & ~bit, deny: deny | bit }
  }
  return { allow, deny: deny & ~bit }
}

export function markClass(allow: number, deny: number, bit: number) {
  if (hasPerm(deny, bit)) return 'deny'
  if (hasPerm(allow, bit)) return 'allow'
  return 'none'
}

export function markLabel(allow: number, deny: number, bit: number) {
  if (hasPerm(deny, bit)) return '−'
  if (hasPerm(allow, bit)) return '+'
  return '·'
}
