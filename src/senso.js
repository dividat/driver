/* Senso bridge
 * Creates a TCP <-> WebSocket bridge for connecting Play to Senso via a TCP connection.
 */
const Connection = require('./persistentConnection')

const DATA_PORT = 55568
const CONTROL_PORT = 55567
const DEFAULT_SENSO_ADDRESS = 'dividat-senso.local'

const log = require('electron-log')

// TODO: Think about removing configuration on Driver side. If Zeroconf works perfectly no configuration should be needed and otherwise it maybe should be stored on Play side.
let config
const constants = require('./constants')
try {
  const Config = require('electron-config')
  config = new Config()
} catch (err) {
  log.warn('Could not load config file.')
  config = {
    get: function (key) {
      return
    },
    set: function (key, value) {
      return
    }
  }
}

const discovery = require('./Senso/discovery')(log)

module.exports = (sensoAddress, recorder) => {
  sensoAddress = sensoAddress || config.get(constants.SENSO_ADDRESS_KEY) || DEFAULT_SENSO_ADDRESS

  // Application level timeout
  let timeout

  var dataConnection = new Connection(sensoAddress, DATA_PORT, 'DATA', log)
  var controlConnection = new Connection(sensoAddress, CONTROL_PORT, 'CONTROL', log)

    // set up recording
  if (recorder) {
    dataConnection.on('data', (data) => {
      recorder.write(data.toString('base64'))
      recorder.write('\n')
    })
  }

  // Monitor data from control connection and keep track of Senso state
  controlConnection.on('data', (raw) => {
    // cancel the timeout
    if (timeout) {
      log.debug('TIMEOUT: Senso alive! Canceling timeout.')
      clearTimeout(timeout)
      timeout = null
    }

    try {
      const response = decodeResponse(raw)
      if (response.error) {
        log.warn('CONTROL: Senso responded with error to a command.')
        log.warn(response)
      } else if (response.status & 0x80000000) {
        log.error('CONTROL: Senso is reporting that an error occured!')
        log.error(response)
      } else {
        log.debug('CONTROL: Monitored Senso status: All good.')
      }
    } catch (e) {
      log.error('CONTROL: Failed to decode response while monitoring Senso status.')
    }
  })

  controlConnection.on('timeout', () => {
    log.debug('TIMEOUT: Checking liveliness by getting Senso status.')
    controlConnection.getSocket().write(GET_STATUS_PACKET)

    // start the application level timeout
    timeout = setTimeout(() => {
      log.warn('TIMEOUT: Connection to Senso seems to have been broken (no response in 2s). Attempting to reconnect...')
      controlConnection.connect()
      dataConnection.connect()
    }, 2000)
  })

  // connect with predifined default
  connect(sensoAddress)

  // Connect to the first Senso discovered
  discovery.once('found', (address) => {
    log.info('mDNS: Auto-connecting to ' + address)
    connect(address)
  })

  function connect (address) {
    log.info('SENSO: Connecting to ' + address)
    dataConnection.connect(address)
    controlConnection.connect(address)
  }

  function onPlayConnection (ws) {
        // Handle a new connection to the application (most probably Play, but could be anything else ... like Manager)
        // argument is a socket.io socket (https://socket.io/docs/server-api/)

    log.info('Play connected (session:', ws.id, ')')

    sendSensoConnection()

    function sendSensoConnection () {
      ws.emit('BridgeMessage', {
        type: 'SensoConnection',
        connected: (controlConnection.connected && dataConnection.connected),
        connection: {
          type: 'IP',
          address: dataConnection.host
        }
      })
    }

    // Create a send function so that it can be cleanly removed from the dataEmitter
    function sendData (data) {
      ws.emit('DataRaw', data)
    }

    function sendControl (data) {
      ws.emit('ControlRaw', data)
    }
    dataConnection.on('data', sendData)
    controlConnection.on('data', sendControl)

    dataConnection.on('connect', sendSensoConnection)
    dataConnection.on('close', sendSensoConnection)
    controlConnection.on('connect', sendSensoConnection)
    controlConnection.on('close', sendSensoConnection)

    // Forward the discovery of (additional) Sensos to Play
    discovery.on('found', (address) => {
      ws.emit('BridgeMessage', {
        type: 'SensoDiscovered',
        connection: {
          type: 'IP',
          address: address
        }
      })
    })

    ws.on('SendControlRaw', (data) => {
      try {
        var socket = controlConnection.getSocket()
        log.debug('CONTROL - sending: ', data)
        if (socket) {
          socket.write(data)
        } else {
          log.warn('Can not send command to Senso, no connection.')
        }
      } catch (e) {
        log.error('Error while handling Command:', e)
      }
    })

    ws.on('BridgeCommand', (command) => {
      try {
        switch (command.type) {
          case 'SensoConnect':
            if (command.connection.type === 'IP' && command.connection.address) {
              config.set(constants.SENSO_ADDRESS_KEY, command.connection.address)
              connect(command.connection.address)
            }
            break
          case 'GetSensoConnection':
            sendSensoConnection()
            break

          default:
            break
        }
      } catch (e) {
        log.error('Error while handling BridgeCommand:', e)
      }
    })

    // handle disconnect
    ws.on('disconnect', () => {
      log.info('WS: Disconnected.')

      dataConnection.removeListener('data', sendData)
      dataConnection.removeListener('connect', sendSensoConnection)
      dataConnection.removeListener('close', sendSensoConnection)

      controlConnection.removeListener('data', sendControl)
      controlConnection.removeListener('connect', sendSensoConnection)
      controlConnection.removeListener('close', sendSensoConnection)
    })
  }

  return onPlayConnection
}

const GET_STATUS_PACKET = (() => {
  // all 0 header
  const header = Buffer.alloc(8)

  // Build the block to be sent
  const block = new ArrayBuffer(4)
  const dataView = new DataView(block)
  // size
  dataView.setUint16(0, 1, true)
  // Block type
  dataView.setUint16(2, 0x00D0, true)

  return Buffer.concat([header, new Buffer(block)])
})()

// Decoding of net response (STD_RESPONSE)
function decodeResponse (raw) {
    // Convert to ArrayBuffer
  const ab = raw.buffer.slice(raw.byteOffset, raw.byteOffset + raw.byteLength)

    // Header
  const headerView = new DataView(ab.slice(0, 8))
  let header = {}
  header.version = headerView.getUint8(0)
  header.numOfBlocks = headerView.getUint8(1)

    // STD_RESPONSE
  const responseView = new DataView(ab.slice(8))
  let response = {}
  response.header = header
  response.len = responseView.getUint16(0, true)
    // blocktype is masked to indicate a response (DATA_TYPE_RESPONSE). Split that out here.
  const blockTypeRaw = responseView.getUint16(2, true)
  response.isResponse = !!(blockTypeRaw & 0x8000)
  response.blockType = blockTypeRaw & 0x0fff

  response.returnCode = responseView.getUint32(4)
  response.status = responseView.getUint32(8)
  response.error = responseView.getUint32(12)

  return response
}
