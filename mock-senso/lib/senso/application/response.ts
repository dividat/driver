import * as Status from './response/status'
import * as DevInfo from './response/device-info'
import * as StandardResponse from './response/standard'

const PROTOCOL_VERSION = 1

enum COMMAND {
  GET_DEV_INFO = 0x00d1,
  GET_STATUS = 0x00d0,
  ENTER_DFU = 0x00f0,
}

const protocolHeader = (function () {
  const numReservedBytes = 6
  const data = [
    PROTOCOL_VERSION,
    1, // numOfBlocks
    ...new Array(numReservedBytes).fill(0),
  ]
  const buffer = Buffer.alloc(data.length)
  data.forEach((byte, offset) => buffer.writeUInt8(byte, offset))
  return buffer
})()

function getBlockHeader(blockLength: number, dataType: number) {
  const buffer = Buffer.alloc(4)
  buffer.writeUInt16LE(blockLength, 0)
  buffer.writeUInt16LE(dataType | 0x8000, 2)
  return buffer
}

function parseCommand(data: Buffer): number[] {
  let numOfBlocks = data.readUInt8(1)
  const body = data.subarray(8)
  const types: number[] = []
  let offset = 0
  while (numOfBlocks > 0 && offset < body.length) {
    const blockLength = body.readUInt16LE(offset)
    const blockType = body.readUInt16LE(offset + 2)
    types.push(blockType)
    offset += 4 + blockLength
    numOfBlocks--
  }
  return types
}

export default function response(data: Buffer): Buffer[] | 'EnterDFU' {
  const commandTypes = parseCommand(data)

  // info
  const version = data.readUInt8(0)
  const commands = commandTypes.map((t) => COMMAND[t] || '<unknown>').join(', ')
  console.log(
    `Received command. Header: Protocol version: ${version}, types(s): ${commands}`,
  )

  if (commandTypes.includes(COMMAND.ENTER_DFU)) {
    return 'EnterDFU'
  }
  return parseCommand(data).map((type) => {
    const [blockHeader, payload] = (function () {
      switch (type) {
        case COMMAND.GET_STATUS:
          return [
            getBlockHeader(Status.BLOCK_LENGTH, COMMAND.GET_STATUS),
            Status.data,
          ]
        case COMMAND.GET_DEV_INFO:
          return [
            getBlockHeader(DevInfo.BLOCK_LENGTH, COMMAND.GET_DEV_INFO),
            DevInfo.data,
          ]
        default:
          return [
            getBlockHeader(StandardResponse.BLOCK_LENGTH, type),
            StandardResponse.data,
          ]
      }
    })()

    return Buffer.concat([protocolHeader, blockHeader, payload])
  })
}
