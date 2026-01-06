// Mock up a Senso data and control server

const { program } = require('commander')
const fs = require('fs')
const split = require('binary-split')
const net = require('net')
const bonjour = require('bonjour')()
const EventEmitter = require('events')

const control = require('./control')

// Define CLI using commander
program
  .description('Mock up a Senso data and control server by replaying recorded data.')
  .argument('[recording-file]', 'path to the recording file', 'rec/senso/zero.dat')
  .option('-s, --speed <number>', 'replay speed multiplier (>1 faster, <1 slower)', parseFloat, 1)
  .option('--once', 'play recording once and exit instead of looping')
  .parse()

const options = program.opts()
const recFile = program.args[0] || 'rec/senso/zero.dat'
let speedFactor = 1 / (options.speed || 1)
let loop = !options.once

async function mockSenso (profile, data) {
  var socket = await listenForConnection('0.0.0.0', 55567)

  // Helper callback that can be removed
  function send (data) {
    socket.write(data)
  }

  data.on('data', send)
  socket.on('data', (incoming) => {
    // Mock a suitable response
    socket.write(control(profile, incoming))
  })

  socket.on('close', () => {
    console.log('Connection closed.')
    data.removeListener('data', send)
    mockSenso(profile, data)
  })

  socket.on('error', (err) => {
    console.log(err)
    data.removeListener('data', send)
    mockSenso(profile, data)
  })
}

function listenForConnection (host, port) {
  return new Promise((resolve, reject) => {
    console.log('Listening on ' + host + ':' + port)
    var server = net.createServer((socket) => {
      console.log('Connection: ' + socket.remoteAddress + ':' + socket.remotePort)

      // disable Nagle
      socket.setNoDelay()

      // Only allow one connection at a time
      server.close()
      resolve(socket)
    }).listen(port, host)
  })
}

// Create a never ending stream of data
function Replayer (recFile) {
  var emitter = new EventEmitter()

  function createStream () {
    var stream = new fs.createReadStream(recFile).pipe(split())

    stream.on('data', (data) => {
      stream.pause()

      var items = data.toString().split(',')
      var msg
      var timeout
      if (items.length === 2) {
        msg = items[1]
        timeout = items[0]
      } else {
        msg = items[0]
        timeout = 20
      }
      var buf = Buffer.from(msg, 'base64')
      emitter.emit('data', buf)

      setTimeout(() => {
        stream.resume()
      }, timeout * speedFactor)
    }).on('end', () => {
      if (loop) {
        console.log('End of the record stream, looping.')
        createStream()
      } else {
        console.log('End of the record stream, exiting.')
        process.exit(0)
      }
    })
  }
  createStream()
  return emitter
}

const profile = {
  serial_number: '31-00000000',
  board_serial_numbers: {
    controller: '30-00000000',
    led_boards: {
      'center': '30-00000001',
      'up': '30-00000002',
      'right': '30-00000003',
      'down': '30-00000004',
      'left': '30-00000005'
    }
  }
}

const dataStream = Replayer(recFile)

// Advertise Senso via mDNS
bonjour.publish({
  name: 'Senso data replayer',
  txt: {ser_no: profile.serial_number, mode: 'Application'},
  type: 'sensoControl',
  port: '55567'})

mockSenso(profile, dataStream)
