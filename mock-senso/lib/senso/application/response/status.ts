import { Buffer } from 'buffer'

const boards: StatusInfoItem[] = Array.from({ length: 6 }, () => ({
  status: [],
  error: [],
  vcc3_3: 3300,
  vcc5: 5000,
  vcc12Mot: 12000,
  vcc12Led: 12000,
  vcc19Led: 19000,
  temperature: 25,
}))

const controllerBoard = boards[0] as StatusInfoItem
controllerBoard.status = ['MEASUREMENT_RUNNING', 'CALIBRATED']
controllerBoard.error = []

enum STATUS_FLAGS {
  BUSY = 0x00000001,
  MEASUREMENT_RUNNING = 0x00000002,
  CALIBRATED = 0x00000004,
  CALIBRATING = 0x00000008,
  NO_SIGNAL = 0x10000000,
  UPGRADING = 0x40000000,
  SYSTEM_ERROR_PRESENT = 0x80000000,
}

enum ERROR_FLAGS {
  GENERAL_ERROR = 0x00000001,
  COMMAND_FAILED = 0x00000002,
  COMMAND_UNKNOWN = 0x00000004,
  COMMUNICATION = 0x00010000,
  LED_BOARD = 0x00020000,
  V3_3_SUPPLY = 0x00100000,
  V5_SUPPLY = 0x00200000,
  V12_MOTOR_SUPPLY = 0x00400000,
  V12_LED_SUPPLY = 0x00800000,
  V19_LED_SUPPLY = 0x01000000,
  OVER_TEMPERATURE = 0x02000000,
}

type StatusFlag = keyof typeof STATUS_FLAGS
type ErrorFlag = keyof typeof ERROR_FLAGS

interface StatusInfoItem {
  status: StatusFlag[]
  error: ErrorFlag[]
  vcc3_3: number
  vcc5: number
  vcc12Mot: number
  vcc12Led: number
  vcc19Led: number
  temperature: number
}

function combineFlags(keys: string[], flags: any): number {
  return keys.reduce((acc, flag) => acc | flags[flag], 0)
}

function serializeStatusInfoItem(item: StatusInfoItem): Buffer {
  const buffer = Buffer.alloc(20)
  let offset = 0

  buffer.writeUInt32LE(combineFlags(item.status, STATUS_FLAGS), offset)
  offset += 4
  buffer.writeUInt32LE(combineFlags(item.error, ERROR_FLAGS), offset)
  offset += 4
  buffer.writeUInt16LE(item.vcc3_3, offset)
  offset += 2
  buffer.writeUInt16LE(item.vcc5, offset)
  offset += 2
  buffer.writeUInt16LE(item.vcc12Mot, offset)
  offset += 2
  buffer.writeUInt16LE(item.vcc12Led, offset)
  offset += 2
  buffer.writeUInt16LE(item.vcc19Led, offset)
  offset += 2
  buffer.writeInt16LE(item.temperature, offset)

  return buffer
}

// Exports
export const BLOCK_LENGTH = 120
export const data = Buffer.concat(boards.map(serializeStatusInfoItem))
