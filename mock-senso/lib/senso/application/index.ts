import net from 'net'
import respond from './response'
import { EventEmitter } from 'stream'
import { promisify } from 'util'
import * as Profile from '../profile'
import * as MDNS from '../mdns'
import * as Replay from '../../replay'

const TCP_PORT = 55567
const DATA_TYPE_SENSOR = 0x80

const server = net.createServer({ noDelay: true })
server.maxConnections = 1
const closeServer = promisify(server.close.bind(server))
const startListening = promisify<number, string>(server.listen.bind(server))

const eventEmitter = new EventEmitter<{ EnterDFU: [] }>()

const mdnsService: MDNS.Service = MDNS.service({
  name: 'Mock Senso',
  type: 'sensoControl',
  protocol: 'tcp',
  txt: {
    ser_no: Profile.serial.device,
    mode: 'Application',
  },
  port: TCP_PORT,
})

export function setup(host: string, replayOpts: Replay.Opts) {
  const replay = Replay.start(replayOpts)
  let socket: net.Socket | undefined

  server.on('connection', (socket) => {
    console.log(`Driver connected`)
    socket = socket

    replay.on('data', (buffer: Buffer) => {
      // Only forward sensor data from replay
      const dataType = buffer.subarray(8).readUInt8(2)
      if (dataType == DATA_TYPE_SENSOR) {
        socket.write(buffer)
      }
    })

    socket.on('data', (buffer) => {
      const responses = respond(buffer)
      switch (responses) {
        case 'EnterDFU': {
          replay.removeAllListeners()
          return eventEmitter.emit('EnterDFU')
        }
        default:
          return responses.forEach((r) => socket.write(r))
      }
    })

    socket.on('error', (e) => {
      console.log(e)
    })

    socket.on('close', (hadError) => {
      console.log(`Driver disconnected ${(hadError && 'with error') || ''}`)
    })
  })

  return {
    on: eventEmitter.on.bind(eventEmitter),
    start: async () => {
      console.log('Starting control mode')
      await startListening(TCP_PORT, host)
      mdnsService.publish()
      replay.resume()
      console.log(`Senso in control mode at ${host}:${TCP_PORT}`)
    },
    stop: async () => {
      console.log('Stopping control mode')
      mdnsService.unpublish()
      socket?.destroy()
      await closeServer()
      replay.pause()
    },
  }
}
