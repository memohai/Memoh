import { mkdtemp, readFile, realpath, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'

import {
  Client,
  Metadata,
  credentials,
  loadPackageDefinition,
  status,
  type ClientDuplexStream,
  type ClientReadableStream,
  type ClientWritableStream,
  type ServiceError,
} from '@grpc/grpc-js'
import { loadSync } from '@grpc/proto-loader'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { grpcMessageLimit, startRuntimeGrpcServer, type RunningRuntimeGrpcServer } from '../src/service'
import type { ExecInput, ExecOutput, FileEntry, WriteRawChunk } from '../src/types'
import { createGrpcWebSocketTestHarness, type GrpcWebSocketTestHarness } from './grpc-websocket-harness'

interface TestClient extends Client {
  Stat(request: { path: string }, callback: UnaryCallback<{ entry: FileEntry }>): void
  Stat(request: { path: string }, metadata: Metadata, callback: UnaryCallback<{ entry: FileEntry }>): void
  ReadFile(request: { path: string, line_offset?: number, n_lines?: number }, callback: UnaryCallback<{
    content: string
    total_lines: number
    binary: boolean
  }>): void
  ReadFile(request: { path: string, line_offset?: number, n_lines?: number }, metadata: Metadata, callback: UnaryCallback<{
    content: string
    total_lines: number
    binary: boolean
  }>): void
  WriteFile(request: { path: string, content: Buffer }, callback: UnaryCallback<Record<string, never>>): void
  WriteFile(request: { path: string, content: Buffer }, metadata: Metadata, callback: UnaryCallback<Record<string, never>>): void
  ListDir(request: { path: string, recursive?: boolean }, callback: UnaryCallback<{
    entries: FileEntry[]
    total_count: number
    truncated: boolean
  }>): void
  ListDir(request: { path: string, recursive?: boolean }, metadata: Metadata, callback: UnaryCallback<{
    entries: FileEntry[]
    total_count: number
    truncated: boolean
  }>): void
  DeleteFile(request: { path: string, recursive?: boolean }, callback: UnaryCallback<Record<string, never>>): void
  DeleteFile(request: { path: string, recursive?: boolean }, metadata: Metadata, callback: UnaryCallback<Record<string, never>>): void
  ReadRaw(request: { path: string }, metadata: Metadata): ClientReadableStream<{ data: Buffer }>
  WriteRaw(metadata: Metadata, callback: UnaryCallback<{ bytes_written: string }>): ClientWritableStream<WriteRawChunk>
  Exec(metadata: Metadata): ClientDuplexStream<ExecInput, ExecOutput>
}

type UnaryCallback<Response> = (error: ServiceError | null, response: Response) => void
type TestClientConstructor = new (
  address: string,
  channelCredentials: ReturnType<typeof credentials.createInsecure>,
  options: Record<string, number>,
) => TestClient

let root: string
let running: RunningRuntimeGrpcServer
let transport: GrpcWebSocketTestHarness
let client: TestClient
const workspaceID = '11111111-1111-4111-8111-111111111111'

beforeEach(async () => {
  root = await mkdtemp(join(tmpdir(), 'memoh-runtime-service-'))
  running = await startRuntimeGrpcServer({
    workspaceBase: root,
    warn: () => undefined,
  })
  transport = await createGrpcWebSocketTestHarness(running)
  const ClientConstructor = loadTestClientConstructor()
  client = new ClientConstructor(
    transport.target,
    credentials.createInsecure(),
    {
      'grpc.max_receive_message_length': grpcMessageLimit,
      'grpc.max_send_message_length': grpcMessageLimit,
    },
  )
})

afterEach(async () => {
  client.close()
  await transport.close()
  await running.close()
  await rm(root, { recursive: true, force: true })
})

describe('ContainerService', () => {
  it('passes readiness and performs file RPCs under /data', async () => {
    const readiness = await unaryUnscoped<{ path: string }, { entry: FileEntry }>(client.Stat.bind(client), { path: '/' })
    expect(readiness.entry.is_dir).toBe(true)

    await unary(client.WriteFile.bind(client), { path: '/data/notes/hello.txt', content: Buffer.from('one\ntwo\n') }, scopeMetadata())
    const result = await unary(client.ReadFile.bind(client), { path: '/data/notes/hello.txt', line_offset: 2, n_lines: 1 }, scopeMetadata())
    expect(result).toEqual({ content: 'two\n', total_lines: 2, binary: false })

    const listing = await unary(client.ListDir.bind(client), { path: '/data', recursive: true }, scopeMetadata())
    expect(listing.entries.map(entry => entry.path)).toContain('notes/hello.txt')

    await expect(unary(client.ReadFile.bind(client), { path: '/data/missing.txt' }, scopeMetadata()))
      .rejects.toMatchObject({ code: status.NOT_FOUND })
    await expect(unary(client.DeleteFile.bind(client), { path: '/data/already-missing.txt' }, scopeMetadata())).resolves.toEqual({})
  })

  it('streams raw files in 64 KiB chunks without changing bytes', async () => {
    const content = Buffer.alloc(150_000, 0x5a)
    const writer = client.WriteRaw(scopeMetadata(), (error, response) => {
      if (error) {
        return
      }
      expect(response.bytes_written).toBe(String(content.length))
    })
    writer.write({ path: '/data/raw.bin', data: content.subarray(0, 70_000) })
    writer.write({ path: '', data: content.subarray(70_000) })
    writer.end()
    await new Promise<void>((resolve, reject) => {
      writer.once('status', statusObject => statusObject.code === status.OK ? resolve() : reject(statusObject))
      writer.once('error', reject)
    })

    const chunks: Buffer[] = []
    const reader = client.ReadRaw({ path: '/data/raw.bin' }, scopeMetadata())
    reader.on('data', chunk => chunks.push(Buffer.from(chunk.data)))
    await streamEnd(reader)
    expect(Buffer.concat(chunks)).toEqual(content)
    expect(chunks.every(chunk => chunk.length <= 64 * 1024)).toBe(true)
  })

  it('runs host shell commands, streams output, and rejects dangerous env', async () => {
    const outputScript = await writeNodeFixture('output.cjs', `
process.stdout.write('out')
process.stderr.write('err')
process.exitCode = 7
`)
    const output = await exec({ command: nodeScriptCommand(outputScript), work_dir: '/data' })
    expect(Buffer.concat(output.filter(frame => frame.stream === 0).map(frame => frame.data)).toString()).toBe('out')
    expect(Buffer.concat(output.filter(frame => frame.stream === 1).map(frame => frame.data)).toString()).toBe('err')
    expect(output.at(-1)).toMatchObject({ stream: 2, exit_code: 7 })

    await expect(exec({ command: 'echo ok', work_dir: '/data', env: ['PATH=/tmp'] }))
      .rejects.toMatchObject({ code: status.PERMISSION_DENIED })
    await expect(exec({ command: 'echo ok', work_dir: '/data', pty: true }))
      .rejects.toMatchObject({ code: status.UNIMPLEMENTED })

    const timeoutScript = await writeNodeFixture('timeout.cjs', 'setInterval(() => {}, 1_000)\n')
    const timedOut = await exec({ command: nodeScriptCommand(timeoutScript), work_dir: '/data', timeout_seconds: 1 })
    expect(timedOut.at(-1)).toMatchObject({ stream: 2 })
    expect(timedOut.at(-1)?.exit_code).not.toBe(0)
    if (process.platform !== 'win32') {
      expect(timedOut.at(-1)?.exit_code).toBe(137)
    }
  })

  it('terminates connection-owned process trees on server close', async () => {
    const pidPath = join(root, 'pid')
    const childScript = await writeNodeFixture('child.cjs', 'setInterval(() => {}, 1_000)\n')
    const parentScript = await writeNodeFixture('parent.cjs', `
const { spawn } = require('node:child_process')
const { writeFileSync } = require('node:fs')

const [pidPath, childPath] = process.argv.slice(2)
const child = spawn(process.execPath, [childPath], { stdio: 'ignore', windowsHide: true })
writeFileSync(pidPath, String(child.pid))
setInterval(() => {}, 1_000)
`)
    const stream = client.Exec(scopeMetadata())
    stream.on('error', () => undefined)
    stream.write({
      command: nodeScriptCommand(parentScript, pidPath, childScript),
      work_dir: '/data',
      timeout_seconds: -1,
    })
    await waitUntil(async () => {
      try {
        return Number(await readFile(pidPath, 'utf8')) > 0
      } catch {
        return false
      }
    })
    const pid = Number(await readFile(pidPath, 'utf8'))
    await running.close()
    await waitUntil(async () => !processExists(pid), 10_000)
    expect(processExists(pid)).toBe(false)
    stream.cancel()
  })

  it('keeps different Bot workspace paths separate on one device connection', async () => {
    const secondID = '22222222-2222-4222-8222-222222222222'
    const first = scopeMetadata(workspaceID, `bots/${workspaceID}`)
    const second = scopeMetadata(secondID, `bots/${secondID}`)

    await Promise.all([
      unary(client.WriteFile.bind(client), { path: '/data/same.txt', content: Buffer.from('first') }, first),
      unary(client.WriteFile.bind(client), { path: '/data/same.txt', content: Buffer.from('second') }, second),
    ])

    const [firstRead, secondRead] = await Promise.all([
      unary(client.ReadFile.bind(client), { path: '/data/same.txt' }, first),
      unary(client.ReadFile.bind(client), { path: '/data/same.txt' }, second),
    ])
    expect(firstRead.content).toBe('first\n')
    expect(secondRead.content).toBe('second\n')

    const cwdScript = await writeNodeFixture('cwd.cjs', 'process.stdout.write(process.cwd())\n')
    const [firstExec, secondExec] = await Promise.all([
      exec({ command: nodeScriptCommand(cwdScript), work_dir: '/data' }, first),
      exec({ command: nodeScriptCommand(cwdScript), work_dir: '/data' }, second),
    ])
    expect(normalizePath(stdout(firstExec))).toBe(normalizePath(await realpath(join(root, 'bots', workspaceID))))
    expect(normalizePath(stdout(secondExec))).toBe(normalizePath(await realpath(join(root, 'bots', secondID))))
  })

  it('lets two Bot scopes explicitly share one workspace path', async () => {
    const secondID = '22222222-2222-4222-8222-222222222222'
    const first = scopeMetadata(workspaceID, 'shared/project')
    const second = scopeMetadata(secondID, 'shared/project')

    await unary(client.WriteFile.bind(client), {
      path: '/data/shared.txt', content: Buffer.from('shared by both'),
    }, first)
    const result = await unary(client.ReadFile.bind(client), { path: '/data/shared.txt' }, second)
    expect(result.content).toBe('shared by both\n')
  })

  it('supports Unicode workspace paths through binary metadata', async () => {
    const metadata = scopeMetadata(workspaceID, '项目/源码')
    await unary(client.WriteFile.bind(client), {
      path: '/data/说明.txt', content: Buffer.from('你好'),
    }, metadata)
    const result = await unary(client.ReadFile.bind(client), { path: '/data/说明.txt' }, metadata)
    expect(result.content).toBe('你好\n')
  })

  it('rejects unscoped and malformed workspace access', async () => {
    await expect(unaryUnscoped(client.ReadFile.bind(client), { path: '/data/nope' }))
      .rejects.toMatchObject({ code: status.PERMISSION_DENIED })
    await expect(exec({ command: 'echo ok', work_dir: '/data' }, new Metadata()))
      .rejects.toMatchObject({ code: status.PERMISSION_DENIED })
    await expect(unary(client.WriteFile.bind(client), { path: '/data/nope', content: Buffer.from('x') }, scopeMetadata(workspaceID, '../escape')))
      .rejects.toMatchObject({ code: status.PERMISSION_DENIED })

    const duplicate = scopeMetadata()
    duplicate.add('x-memoh-workspace-id', '22222222-2222-4222-8222-222222222222')
    await expect(unary(client.WriteFile.bind(client), { path: '/data/nope', content: Buffer.from('x') }, duplicate))
      .rejects.toMatchObject({ code: status.PERMISSION_DENIED })

    await expect(unary(
      client.WriteFile.bind(client),
      { path: '/data/nope', content: Buffer.from('x') },
      scopeMetadata('AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA', 'bots/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'),
    )).rejects.toMatchObject({ code: status.PERMISSION_DENIED })

    const invalidUTF8 = scopeMetadata()
    invalidUTF8.set('x-memoh-workspace-path-bin', Buffer.from([0xff]))
    await expect(unary(client.WriteFile.bind(client), { path: '/data/nope', content: Buffer.from('x') }, invalidUTF8))
      .rejects.toMatchObject({ code: status.PERMISSION_DENIED })

    await expect(exec({ command: 'echo ok', work_dir: tmpdir() }))
      .rejects.toMatchObject({ code: status.PERMISSION_DENIED })
  })

  it('requires valid workspace scope metadata for an empty WriteRaw stream', async () => {
    await expect(emptyWriteRaw(new Metadata()))
      .rejects.toMatchObject({ code: status.PERMISSION_DENIED })

    const duplicate = scopeMetadata()
    duplicate.add('x-memoh-workspace-path-bin', Buffer.from(`bots/${workspaceID}`, 'utf8'))
    await expect(emptyWriteRaw(duplicate))
      .rejects.toMatchObject({ code: status.PERMISSION_DENIED })

    await expect(emptyWriteRaw(scopeMetadata())).resolves.toEqual({ bytes_written: '0' })
  })

})

function loadTestClientConstructor(): TestClientConstructor {
  const proto = fileURLToPath(new URL('../../../internal/workspace/bridgepb/bridge.proto', import.meta.url))
  const definition = loadSync(proto, {
    keepCase: true,
    longs: String,
    enums: Number,
    defaults: true,
    oneofs: true,
  })
  const descriptor = loadPackageDefinition(definition) as unknown as {
    bridgepb: { ContainerService: TestClientConstructor }
  }
  return descriptor.bridgepb.ContainerService
}

function unary<Request, Response>(
  method: (request: Request, metadata: Metadata, callback: UnaryCallback<Response>) => void,
  request: Request,
  metadata: Metadata,
): Promise<Response> {
  return new Promise((resolve, reject) => {
    method(request, metadata, (error, response) => error ? reject(error) : resolve(response))
  })
}

function unaryUnscoped<Request, Response>(
  method: (request: Request, callback: UnaryCallback<Response>) => void,
  request: Request,
): Promise<Response> {
  return new Promise((resolve, reject) => {
    method(request, (error, response) => error ? reject(error) : resolve(response))
  })
}

function streamEnd(stream: ClientReadableStream<unknown>): Promise<void> {
  return new Promise((resolve, reject) => {
    stream.once('end', resolve)
    stream.once('error', reject)
  })
}

function exec(first: ExecInput, metadata = scopeMetadata()): Promise<ExecOutput[]> {
  const stream = client.Exec(metadata)
  const frames: ExecOutput[] = []
  stream.on('data', frame => frames.push(frame))
  stream.write(first)
  stream.end()
  return new Promise((resolve, reject) => {
    stream.once('end', () => resolve(frames))
    stream.once('error', reject)
  })
}

function emptyWriteRaw(metadata: Metadata): Promise<{ bytes_written: string }> {
  return new Promise((resolve, reject) => {
    const writer = client.WriteRaw(metadata, (error, response) => error ? reject(error) : resolve(response))
    // Callback-style grpc-js streams also emit their terminal error. The
    // callback owns the assertion, so keep the EventEmitter error handled.
    writer.on('error', () => undefined)
    writer.end()
  })
}

function scopeMetadata(id = workspaceID, path = `bots/${id}`): Metadata {
  const metadata = new Metadata()
  metadata.set('x-memoh-workspace-id', id)
  metadata.set('x-memoh-workspace-path-bin', Buffer.from(path, 'utf8'))
  return metadata
}

function stdout(frames: ExecOutput[]): string {
  return Buffer.concat(frames.filter(frame => frame.stream === 0).map(frame => frame.data)).toString()
}

async function writeNodeFixture(name: string, source: string): Promise<string> {
  const path = join(root, name)
  await writeFile(path, source)
  return path
}

function nodeScriptCommand(scriptPath: string, ...args: string[]): string {
  return `node ${[scriptPath, ...args].map(quoteShellArgument).join(' ')}`
}

function quoteShellArgument(value: string): string {
  if (process.platform === 'win32') {
    if (value.includes('"')) {
      throw new Error('Windows test paths must not contain quotes')
    }
    return `"${value}"`
  }
  const escaped = value.replaceAll('\u0027', '\u0027"\u0027"\u0027')
  return `'${escaped}'`
}

function normalizePath(value: string): string {
  const path = value.trim()
  return process.platform === 'win32' ? path.toLowerCase() : path
}

async function waitUntil(predicate: () => Promise<boolean>, timeout = 5_000): Promise<void> {
  const deadline = Date.now() + timeout
  while (Date.now() < deadline) {
    if (await predicate()) {
      return
    }
    await new Promise(resolve => setTimeout(resolve, 25))
  }
  throw new Error('condition did not become true')
}

function processExists(pid: number): boolean {
  try {
    process.kill(pid, 0)
    return true
  } catch {
    return false
  }
}
