interface StandardResponse {
  returnCode: number
  status: number
  error: number
}

function serialize(response: StandardResponse): Buffer {
  const buffer = Buffer.alloc(BLOCK_LENGTH)
  buffer.writeUInt32LE(response.returnCode)
  buffer.writeUInt32LE(response.status)
  buffer.writeUInt32LE(response.error)
  return buffer
}

export const BLOCK_LENGTH = 24
export const data = serialize({ returnCode: 0, status: 0, error: 0 })
