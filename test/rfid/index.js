/* eslint-env mocha */
const { wait, startDriver, connectWS, getJSON, expectEvent, waitForEndpoint } = require('../utils')
const expect = require('chai').expect

// TESTS

describe('Basic functionality', () => {
  var driver
  var rfid = {}

  beforeEach(async () => {
  // Start driver
    var code = 0
    driver = startDriver().on('exit', (c) => {
      code = c
    })
    await waitForEndpoint('http://127.0.0.1:8382/rfid');
    expect(code).to.be.equal(0)
    driver.removeAllListeners()
  })

  afterEach(() => {
    driver.kill()
  })

  it('Can retrieve current reader list.', async function () {
    this.timeout(500)

    const response = await getJSON('http://127.0.0.1:8382/rfid/readers')
    expect(response.readers).to.be.an('array')
  })

  it('Can connect to the RFID endpoint.', async function () {
    this.timeout(500)

    await connectWS('ws://127.0.0.1:8382/rfid')
  })

})
