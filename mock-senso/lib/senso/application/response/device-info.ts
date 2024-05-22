import * as Profile from '../../profile'

function devInfoItem(serial: string) {
  const devInfoItem = Buffer.alloc(24) // swVersion (4) + hwVersion (4) + serial_number (16)
  let offset = 0

  // software version
  devInfoItem.writeInt8(Profile.defaultVersion.fix, offset)
  offset++
  devInfoItem.writeInt8(Profile.defaultVersion.feature, offset)
  offset++
  devInfoItem.writeInt8(Profile.defaultVersion.minor, offset)
  offset++
  devInfoItem.writeInt8(Profile.defaultVersion.major, offset)
  offset++

  // hardware version
  devInfoItem.writeUInt32LE(0, 12)
  offset += 4

  // serial number
  devInfoItem.write(serial, offset, 16, 'ascii')

  return devInfoItem
}

const deviceSerialNumber = Buffer.alloc(16)
deviceSerialNumber.write(Profile.serial.device, 0, 16)

// Exports
export const BLOCK_LENGTH = 168
export const data = Buffer.concat([
  deviceSerialNumber,

  // mac address
  Buffer.alloc(1, 222),
  Buffer.alloc(1, 173),
  Buffer.alloc(1, 190),
  Buffer.alloc(1, 239),
  Buffer.alloc(1, 202),
  Buffer.alloc(1, 254),

  // reserved bytes
  Buffer.alloc(2, 0),

  // controller board
  devInfoItem(Profile.serial.board.controller),

  // led boards
  devInfoItem(Profile.serial.board.center),
  devInfoItem(Profile.serial.board.up),
  devInfoItem(Profile.serial.board.right),
  devInfoItem(Profile.serial.board.down),
  devInfoItem(Profile.serial.board.left),
])
