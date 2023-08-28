import { createEcmaScriptPlugin } from '@bufbuild/protoplugin';
import {
  GeneratedFile,
  Schema,
  findCustomMessageOption,
} from '@bufbuild/protoplugin/ecmascript';
import { DescMethod, MethodKind } from '@bufbuild/protobuf';
import { version } from '../package.json';
import { HttpRule } from '../../pkg/opni/generated/google/api/http_pb';

export default createEcmaScriptPlugin({
  name:    'service-generator',
  version: `v${ String(version) }`,
  generateTs,
});

function generateTs(schema: Schema) {
  schema.files.forEach((file) => {
    const f = schema.generateFile(`${ file.name }_svc.ts`);

    f.preamble(file);

    file.services.forEach((service) => {
      service.methods.forEach(method => printMethod(f, method));
    });
  });
}

function printMethod(f: GeneratedFile, method: DescMethod) {
  const m = findCustomMessageOption(method, 72295728, HttpRule);
  const input = f.import(method.input);
  const output = f.import(method.output);

  const _axios = f.import('axios', 'axios');
  const _Socket = f.import('Socket', '@pkg/opni/utils/socket');
  const _EVENT_CONNECTED = f.import('EVENT_CONNECTED', '@shell/utils/socket');
  const _EVENT_CONNECT_ERROR = f.import('EVENT_CONNECT_ERROR', '@shell/utils/socket');
  const _EVENT_DISCONNECT_ERROR = f.import('EVENT_DISCONNECT_ERROR', '@shell/utils/socket');
  const _EVENT_MESSAGE = f.import('EVENT_MESSAGE', '@shell/utils/socket');

  const inputName = input.name === 'Empty' ? '' : 'input: ';
  // const inputType = input.name === 'Empty' ? '' : input.name;
  // const returnType = output.name === 'Empty' ? 'void' : output.name;
  const data = inputName ? `,\n    data: input.toJson()` : '';

  switch (method.methodKind) {
  case MethodKind.Unary:
    f.print(`
  export async function ${ method.name }(`, inputName, input, `): Promise<`, output, `> {
    const result = (await `, _axios, `.request({
      method: "${ m?.pattern.case || 'get' }",
      url: '/opni-api/${ method.parent.name }${ m?.pattern.value || '' }'${ data }
    })).data;
    return `, output, `.fromJson(result);
  }\n`);
    break;
  case MethodKind.ClientStreaming:
    // not implemented
    break;
  case MethodKind.BiDiStreaming:
    // not implemented
    break;
  case MethodKind.ServerStreaming:
    f.print(`
  export function ${ method.name }(input: `, input, `, callback: (data: `, output, `) => void): () => Promise<any> {
    const socket = new `, _Socket, `('/opni-api/${ method.parent.name }${ m?.pattern.value || '' }', true);
    socket.frameTimeout = null;
    socket.addEventListener(`, _EVENT_MESSAGE, `, (e: any) => {
      const event = e.detail;
      if (event.data) {
        callback(`, output, `.fromJson(JSON.parse(event.data)));
      }
    });
    socket.addEventListener(`, _EVENT_CONNECTED, `, () => {
      socket.send(input.toJsonString());
    }, { once: true });
    socket.addEventListener(`, _EVENT_CONNECT_ERROR, `, (e) => {
      console.error(e);
    })
    socket.addEventListener(`, _EVENT_DISCONNECT_ERROR, `, (e) => {
      console.error(e);
    })
    socket.connect();
    return () => {
      return socket.disconnect(null);
    };
  }\n`);
    break;
  }
}
