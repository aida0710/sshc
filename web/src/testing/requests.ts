export function sentJson(init: RequestInit): unknown {
  const { body } = init;
  if (typeof body !== "string") {
    throw new Error(`the request body is ${typeof body}, not a JSON string`);
  }
  return JSON.parse(body);
}
