export interface Version {
  major: number
  minor: number
  patch: number
  feature: number
  fix: number
}

export const defaultVersion = {
  major: 3,
  minor: 3,
  patch: 3,
  feature: 3,
  fix: 3,
}
export const serial = {
  device: '10-00000000',
  board: {
    controller: '10-00000001',
    center: '10-00000002',
    up: '10-00000003',
    right: '10-00000004',
    down: '10-00000005',
    left: '10-00000006',
  },
}
