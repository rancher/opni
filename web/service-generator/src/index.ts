import { createEcmaScriptPlugin } from '@bufbuild/protoplugin';
import { GeneratedFile, Schema, findCustomMessageOption } from '@bufbuild/protoplugin/ecmascript';
import { DescMethod } from '@bufbuild/protobuf';
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

    f.print('import axios from "axios";\n');

    file.services.forEach((service) => {
      service.methods.forEach(method => printMethod(f, method));
    });
  });
}

function printMethod(f: GeneratedFile, method: DescMethod) {
  const m = findCustomMessageOption(method, 72295728, HttpRule);
  const input = f.import(method.input);
  const output = f.import(method.output);
  const inputName = input.name === 'Empty' ? '' : 'input: ';
  const inputType = input.name === 'Empty' ? '' : input;
  const returnType = output.name === 'Empty' ? 'void' : output;
  const data = inputName ? `,\n    data: input.toJson()` : '';

  f.print(`export async function ${ method.name }(`, inputName, inputType, `): Promise<`, returnType, `> {
   const result = (await axios.request({
    method: "${ m?.pattern.case || 'get' }",
    url: '/opni-api/${ method.parent.name }${ m?.pattern.value || '' }'${ data }
   })).data;

   return `, output, `.fromJson(result);
}\n`);
}
