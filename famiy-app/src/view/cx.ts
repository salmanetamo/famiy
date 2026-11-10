/** Joins class names, skipping the ones that are missing or off (CSS-module lookups may be undefined). */
export function cx(...names: (string | false | undefined)[]): string {
  return names.filter((name): name is string => typeof name === 'string' && name !== '').join(' ');
}
