import * as fs from 'fs';
import * as path from 'path';
import { createEcmaScriptPlugin } from '@bufbuild/protoplugin';
import {
  GeneratedFile,
  Schema,
  findCustomMessageOption,
} from '@bufbuild/protoplugin/ecmascript';
import { DescMethod, MethodKind, DescService } from '@bufbuild/protobuf';
import { HttpRule } from '../../pkg/opni/generated/google/api/http_pb';
import { version } from '../package.json';

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
      service.methods.forEach(method => printMethod(f, method, service));
    });
  });
}

function printMethod(f: GeneratedFile, method: DescMethod, service: DescService) {
  const m = findCustomMessageOption(method, 72295728, HttpRule);
  const input = f.import(method.input);
  const output = f.import(method.output);

  const _axios = f.import('axios', '@pkg/opni/utils/axios');
  const _Socket = f.import('Socket', '@pkg/opni/utils/socket');
  const _EVENT_CONNECTED = f.import('EVENT_CONNECTED', '@shell/utils/socket');
  const _EVENT_CONNECTING = f.import('EVENT_CONNECTING', '@shell/utils/socket');
  const _EVENT_CONNECT_ERROR = f.import('EVENT_CONNECT_ERROR', '@shell/utils/socket');
  const _EVENT_DISCONNECT_ERROR = f.import('EVENT_DISCONNECT_ERROR', '@shell/utils/socket');
  const _EVENT_MESSAGE = f.import('EVENT_MESSAGE', '@shell/utils/socket');

  const inputIsEmpty = input.name === 'Empty';
  const outputIsEmpty = output.name === 'Empty';
  // const inputType = input.name === 'Empty' ? '' : input.name;
  // const returnType = output.name === 'Empty' ? 'void' : output.name;
  // const transformRequest = inputIsEmpty ? '' : `\n    transformRequest: req => req.toJsonString(),`;
  // const transformResponse = outputIsEmpty ? '' : [`\n    transformResponse: resp => `, output, `.fromBinary(new Uint8Array(resp)),`];
  const data = inputIsEmpty ? '' : `,\n    data: input?.toBinary() as ArrayBuffer`;
  const urlPath = (m?.pattern.value as any || '').replaceAll('{', '${input.');
  const potentialModelPath = output ? path.join('./web/pkg/opni/models', service.name, `${ output.name }.ts`) : '';
  const modelFound = potentialModelPath && fs.existsSync(potentialModelPath);
  const modelImport = modelFound ? f.import(output.name, `@pkg/opni/models/${ service.name }/${ output.name }`) : null;

  const inputLogMessage = inputIsEmpty ? '' : `
    if (input) {
      console.info('Here is the input for a request to ${ service.name }-${ method.name }:', input);
    }
  `;

  const responseTranform = outputIsEmpty ? [`rawResponse;`] : [output, `.fromBinary(new Uint8Array(rawResponse));`];
  const returnValue = modelImport ? [`return new `, modelImport, `(response);`] : [`return response;`];

  switch (method.methodKind) {
  case MethodKind.Unary:
    f.print(`
export async function ${ method.name }(`, ...(inputIsEmpty ? [] : ['input: ', input]), `): Promise<`, outputIsEmpty ? 'void' : modelImport || output, `> {
  try {
    ${ inputLogMessage }
    const rawResponse = (await `, _axios, `.request({
      method: '${ m?.pattern.case || 'get' }',
      responseType: 'arraybuffer',
      headers: {
        'Content-Type': 'application/octet-stream',
        'Accept': 'application/octet-stream',
      },
      url: \`/opni-api/${ method.parent.name }${ urlPath }\`${ data }
    })).data;

    const response = `, ...responseTranform, `
    console.info('Here is the response for a request to ${ service.name }-${ method.name }:', response);
    `, ...returnValue, `
  } catch (ex: any) {
    if (ex?.response?.data) {
      const s = String.fromCharCode.apply(null, Array.from(new Uint8Array(ex?.response?.data)));
      console.error(s);
    }
    throw ex;
  }
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
  Object.assign(socket, { frameTimeout: null })
  socket.addEventListener(`, _EVENT_MESSAGE, `, (e: any) => {
    const event = e.detail;
    if (event.data) {
      callback(`, output, `.fromBinary(new Uint8Array(event.data)));
    }
  });
  socket.addEventListener(`, _EVENT_CONNECTING, `, () => {
    if (socket.socket) {
      socket.socket.binaryType = 'arraybuffer';
    }
  });
  socket.addEventListener(`, _EVENT_CONNECTED, `, () => {
    socket.send(input.toBinary());
  });
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
