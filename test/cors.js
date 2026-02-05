/* eslint-env mocha */

const { wait, getJSON, startDriver, connectWS, waitForEndpoint } = require('./utils')
const expect = require('chai').expect

const httpEndpoints = [
  'http://127.0.0.1:8382',
  'http://127.0.0.1:8382/log',
  'http://127.0.0.1:8382/rfid/readers'
]

const wsEndpoints = [
  'ws://127.0.0.1:8382/senso',
  'ws://127.0.0.1:8382/flex'
]

const permissibleOrigins = [ 'https://test-origin.xyz', 'http://127.0.0.1:8000' ]
const untrustedOrigin = 'https://foreign-origin.xyz'

let driver

beforeEach(async () => {
  let code = 0
  driver = startDriver(...permissibleOrigins.flatMap(origin => [ '--permissible-origin', origin ])).on('exit', (c) => {
    code = c
  })
  await waitForEndpoint('http://127.0.0.1:8382/');
  expect(code).to.be.equal(0)
  driver.removeAllListeners()
})

afterEach(() => {
  driver.kill()
})

// No `Origin` header

it('can make HTTP requests with no origin set', async () => {
  return Promise.all(httpEndpoints.map(url =>
    fetch(url).then((response) => { expect(response.status, `GET ${url}`).to.equal(200) })
  ))
})

it('can connect to WebSocket endpoints with no origin set', async () => {
  return Promise.all(wsEndpoints.map(async (url) => {
    let connected = await connectWS(url).then(_ => true).catch(_ => false)
    expect(connected, `WS ${url}`).to.be.true
  }))
})

// Known `Origin` header

it('can make HTTP requests with known origin set', async () => {
  return Promise.all(httpEndpoints.flatMap(url =>
    permissibleOrigins.map(origin =>
      fetch(url, { headers: { Origin: origin } }).then((response) => {
        expect(response.status, `GET ${url} (Origin: ${origin})`).to.equal(200)
        expect(response.headers.get('Access-Control-Allow-Origin'), `Access-Control-Allow-Origin ${url}`).to.equal(origin)
        expect(response.headers.get('Access-Control-Allow-Private-Network'), `Access-Control-Allow-Private-Network ${url}`).to.equal('true')
      })
    )
  ))
})

it('can connect to WebSocket endpoints with known origin set', async () => {
  return Promise.all(wsEndpoints.flatMap(url =>
    permissibleOrigins.map(async (origin) => {
      let connected = await connectWS(url, { headers: { Origin: origin } }).then(_ => true).catch(_ => false)
      expect(connected, `WS ${url} (Origin: ${origin})`).to.be.true
    })
  ))
})

// Unknown `Origin` header

it('can not make HTTP requests with unknown origin set', async () => {
  return Promise.all(httpEndpoints.map(url =>
    fetch(url, { headers: { Origin: untrustedOrigin } }).then((response) => {
      expect(response.status, `GET ${url}`).to.equal(403)
    })
  ))
})

it('can not connect to WebSocket endpoints with unknown origin set', async () => {
  return Promise.all(wsEndpoints.map(async (url) => {
    let connected = await connectWS(url, { headers: { Origin: untrustedOrigin } }).then(_ => true).catch(_ => false)
    expect(connected, `WS ${url}`).to.be.false
  }))
})
