import { createEcmaScriptPlugin } from '@bufbuild/protoplugin';
import { GeneratedFile, Schema, findCustomMessageOption } from '@bufbuild/protoplugin/ecmascript';
import { DescMethod } from '@bufbuild/protobuf';
import { version } from '../package.json';
import { HttpRule } from './httprule_pb';

export default createEcmaScriptPlugin({
  name:    'protoc-gen-opni-services',
  version: `v${ String(version) }`,
  generateTs,
});

function generateTs(schema: Schema) {
  schema.files.forEach((file) => {
    const f = schema.generateFile(`${ file.name }_svc.ts`);

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

  f.print(`export async function ${ method.name }(input:`, input, `): Promise<`, output, `> {
   const result = (await axios.request({
    method: "${ m?.pattern.case || 'get' }",
    url: '/opni/${ method.parent.name }${ m?.pattern.value || '' }',
    data: input.toJson()
   })).data;

   return `, output, `.fromJson(result);
}\n`);
}
