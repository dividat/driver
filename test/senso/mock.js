// mock Senso

const { Writable } = require('stream')
const EventEmitter = require('events')
const net = require('net')

const HOST = '127.0.0.1'
const PORT = 55567

function createChannel () {
  const channel = new EventEmitter()

  var closed = false

  channel.stream = new Writable({
    write: (chunk, encoding, cb) => {
      cb = cb || (() => {})
      if (channel._connection) {
        channel._connection.write(chunk)
      }
      return cb()
    },
    final: (cb) => {
      return cb()
    }
  })

  channel._server = net.createServer()
    .listen(PORT, HOST)
    .on('listening', () => {
      channel.emit('listening')
    })
    .on('error', (e) => {
      channel.emit('error', e)
    })
    .on('connection', (c) => {
      channel._connection = c
      channel._server.close()
      channel.emit('connection', c)

      c.on('close', () => {
        channel._connection = null
        if (!closed) {
          channel._server.listen(PORT, HOST)
        }
      })
    })

  channel.close = () => {
    closed = true
    if (channel._server) {
      channel._server.close()
    }
  }

  return channel
}

module.exports = createChannel;
