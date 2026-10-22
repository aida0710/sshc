// ConfigOperation は、SSH Config の画面が走らせるプレビューと書き込みである。一度に一つだけ
// 走らせ、待っている間は、押したボタンだけに回転の印を出し、ほかの操作のボタンを止める。
export type ConfigOperation =
  | "preview"
  | "save"
  | "rename"
  | "delete"
  | "createFile"
  | "createDirectory"
  | "deleteDirectory";
