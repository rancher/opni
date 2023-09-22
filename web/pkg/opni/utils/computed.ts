import { Duration } from '@bufbuild/protobuf';

export function createComputedTime(path: string, factor: number = 1, defaultValue: number = 0) {
  return {
    get() {
      return Number.parseInt(conditionalGet(this, path) || `${ defaultValue }`) / factor;
    },

    set(value: number) {
      const parts = path.split('.');
      const last = parts.splice(-1, 1)[0];
      const prefix = parts.join('.');

      (this as any).$set(conditionalGet(this, prefix), last, `${ (value || defaultValue) * factor }s`);
    }
  };
}

export function createComputedDuration(path: string, factor: number = 1, defaultValue: number = 0) {
  return {
    get() {
      const duration: Duration = conditionalGet(this, path) || new Duration({ seconds: BigInt(defaultValue) });

      return Number.parseInt(duration.seconds.toString()) / factor;
    },

    set(value: number) {
      const parts = path.split('.');
      const last = parts.splice(-1, 1)[0];
      const prefix = parts.join('.');

      (this as any).$set(conditionalGet(this, prefix), last, new Duration({ seconds: BigInt((value || defaultValue) * factor) }));
    }
  };
}

function conditionalGet(source: any, key: string) {
  const parts = key.split('.');

  for (const part of parts) {
    source = source?.[part];
  }

  return source;
}
