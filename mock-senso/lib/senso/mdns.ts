import { spawn, ChildProcess } from 'child_process'
import os from 'os'

export interface TextRecord {
  mode: 'Application' | 'Bootloader'
  ser_no: string
}

export interface ServiceConfig {
  name: string
  type: 'sensoControl' | 'sensoUpdate'
  port: number
  protocol: 'tcp' | 'udp'
  txt: TextRecord
}

export interface Service {
  publish: () => void
  unpublish: () => void
}

export function service(config: ServiceConfig): Service {
  let childProcess: ChildProcess | null = null

  const txtArgs = Object.entries(config.txt).map(
    ([key, value]) => `${key}=${value}`,
  )
  const serviceType = `_${config.type}._${config.protocol}`
  const platform = os.platform()

  return {
    publish: () => {
      childProcess = (function () {
        switch (platform) {
          case 'darwin': {
            return spawn('dns-sd', [
              '-R',
              config.name,
              serviceType,
              'local',
              `${config.port}`,
              ...txtArgs,
            ])
          }
          case 'linux':
            return spawn('avahi-publish-service', [
              config.name,
              serviceType,
              `${config.port}`,
              ...txtArgs,
            ])
          default:
            console.log('Unsupported platform')
            process.exit(1)
        }
      })()

      childProcess.on('spawn', () => {
        console.log('Published mDNS service', {
          name: config.name,
          type: serviceType,
          txt: config.txt,
          port: config.port,
        })
      })

      childProcess.on('error', (e) => {
        console.log('Could not publish mDNS service')
        console.error(e)
        process.exit(1)
      })
    },
    unpublish: () => {
      if (childProcess) {
        childProcess.on('exit', () => {
          console.log(`Unpublished ${serviceType}`)
          childProcess = null
        })
        childProcess.kill('SIGTERM')
      }
    },
  }
}
