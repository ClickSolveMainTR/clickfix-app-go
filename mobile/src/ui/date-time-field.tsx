import { useState } from 'react';
import { Input } from './input';
export function DateTimeField({
  label,
  value,
  onChange,
}: {
  label: string;
  value: Date;
  onChange: (date: Date) => void;
  min?: Date;
}) {
  const [text, setText] = useState(value.toISOString());
  return (
    <Input
      label={label}
      value={text}
      onChangeText={(s) => {
        setText(s);
        const date = new Date(s);
        if (Number.isFinite(date.getTime())) onChange(date);
      }}
    />
  );
}
