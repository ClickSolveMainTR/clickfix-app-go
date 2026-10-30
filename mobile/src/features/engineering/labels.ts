export function humanLabel(value: string) {
  const label = value.replace(/[-_]+/g, ' ').trim();
  return label ? label[0].toLocaleUpperCase('tr') + label.slice(1) : label;
}
