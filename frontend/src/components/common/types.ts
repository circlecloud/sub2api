/**
 * Common component types
 */

export interface SortDescriptor {
  key: string
  order: 'asc' | 'desc'
}

export interface Column {
  key: string
  label: string
  sortable?: boolean
  class?: string
  formatter?: (value: any, row: any) => string
}
