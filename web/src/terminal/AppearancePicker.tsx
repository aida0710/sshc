import { control } from "../ui/form";

type Choice = { readonly name: string; readonly label: string };

type AppearancePickerProps = {
  choices: readonly Choice[];
  value: string;
  onChange: (next: string) => void;
  unchosen: string;
};

export function AppearancePicker({ choices, value, onChange, unchosen }: AppearancePickerProps) {
  return (
    <select className={control} value={value} onChange={(event) => onChange(event.target.value)}>
      <option value="">{unchosen}</option>
      {choices.map((choice) => (
        <option key={choice.name} value={choice.name}>
          {choice.label}
        </option>
      ))}
    </select>
  );
}
