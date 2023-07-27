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

function conditionalGet(source: any, key: string) {
  const parts = key.split('.');

  for (const part of parts) {
    source = source?.[part];
  }

  return source;
}
