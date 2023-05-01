export function isStandalone() {
  return process.env.isStandalone;
}

export function choosePageLayout() {
  return isStandalone() ? 'standalone' : 'plain';
}
