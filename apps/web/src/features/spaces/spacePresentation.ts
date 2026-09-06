import type { SpaceCreate } from '../../api/generated/types.gen'

export const spaceIcons: Record<SpaceCreate['type'], string> = {
  DAILY: 'wallet',
  BILLS: 'receipt',
  HOUSEHOLD: 'house-line',
  SAVINGS: 'vault',
  GOAL: 'target',
  CUSTOM: 'columns',
}
