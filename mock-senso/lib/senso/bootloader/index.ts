import dgram from 'dgram'
import { promisify } from 'util'
import EventEmitter from 'events'
import * as MDNS from '../mdns'
import * as Profile from '../profile'

// Use a higher port number rather than the standard 69
// to avoid permission issues on linux.
const TFTP_PORT = 6969
const emitter = new EventEmitter<{ close: [] }>()

const mdnsService: MDNS.Service = MDNS.service({
  name: 'Mock Senso',
  txt: {
    ser_no: Profile.serial.device,
    mode: 'Bootloader',
  },
  type: 'sensoUpdate',
  protocol: 'udp',
  port: TFTP_PORT,
})

function receiveFirmware(socket: dgram.Socket) {
  socket.on('listening', () => {
    const { address, port } = socket.address()
    console.log(`TFTP server listening ${address}:${port}`)
  })
  return new Promise<void>((resolve, reject) => {
    socket.on('error', (err) => {
      socket.close()
      reject(err)
    })

    let expectedBlockNumber = 0
    let totalBytesReceived = 0

    socket.on('message', async (msg, rinfo) => {
      const opcode = msg.readUInt16BE(0)
      switch (opcode) {
        case 2: // WRQ
          console.log(`WRQ received from ${rinfo.address}:${rinfo.port}.`)
          expectedBlockNumber = 1
          totalBytesReceived = 0
          sendAck(0, rinfo, socket)
          break
        case 3: // DATA
          const blockNumber = msg.readUInt16BE(2)
          if (blockNumber === expectedBlockNumber) {
            const dataLength = msg.length - 4 // Subtract header bytes
            totalBytesReceived += dataLength
            const ackSent = sendAck(blockNumber, rinfo, socket)

            if (dataLength < 512) {
              console.log(
                `Last data packet received. Total bytes received: ${totalBytesReceived}`,
              )
              await ackSent
              resolve()
            } else {
              expectedBlockNumber++
            }
          }
          break
        default:
          console.log(`Unsupported opcode received: ${opcode}`)
      }
    })
  })
}

function sendAck(
  blockNumber: number,
  rinfo: dgram.RemoteInfo,
  server: dgram.Socket,
) {
  const ackBuffer = Buffer.alloc(4)
  ackBuffer.writeUInt16BE(4, 0) // Opcode for ACK
  ackBuffer.writeUInt16BE(blockNumber, 2) // Block number
  return new Promise<void>((resolve, reject) => {
    server.send(
      ackBuffer,
      0,
      ackBuffer.length,
      rinfo.port,
      rinfo.address,
      (err) => {
        if (err) {
          console.error(`Error sending ACK for block ${blockNumber}: ${err}`)
          reject(err)
        } else {
          resolve()
        }
      },
    )
  })
}

function bindSocket(socket: dgram.Socket, host: string): Promise<void> {
  return new Promise((resolve, reject) => {
    function handleError(e: Error) {
      reject(e)
    }
    socket.on('error', handleError)
    socket.bind(TFTP_PORT, host, () => {
      socket.removeListener('error', handleError)
      resolve()
    })
  })
}

export function setup(host: string) {
  return {
    onClose: (fn: () => any) => emitter.on('close', fn),
    start: async () => {
      console.log('Entering Bootloader mode')
      const socket = dgram.createSocket('udp4')
      try {
        await bindSocket(socket, host)
      } catch (e) {
        console.error('Could not setup TFTP server: ', e)
        emitter.emit('close')
        return
      }

      const closeSocket = promisify(socket.close.bind(socket))

      mdnsService.publish()

      receiveFirmware(socket)
        .catch((e) => console.error(`TFTP server error:\n${e.stack}`))
        .then(async () => {
          console.log('Stopping Bootloader mode')
          socket.removeAllListeners()
          await closeSocket().catch((e) =>
            console.log(`Failed to close dgram socket: ${e.message}`),
          )
          mdnsService.unpublish()
          emitter.emit('close')
        })
    },
  }
}
