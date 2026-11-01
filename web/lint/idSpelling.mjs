// 識別子は API の JSON と同じ `Id` の綴りに揃える。`sessionID` と `sessionId` が
// 混ざると、grep で片方しか見つからない。
// Oxlint には typescript-eslint の naming-convention に当たる規則が無いため、ここに書く。
const uppercaseId = /[a-z]IDs?$/;

// 名前を決める位置と、その名前が入る子の鍵。読むだけの位置（`external.remoteID` など）は咎めない。
const namingKeys = {
  VariableDeclarator: "id",
  FunctionDeclaration: "id",
  FunctionExpression: "id",
  TSDeclareFunction: "id",
  ClassDeclaration: "id",
  ClassExpression: "id",
  PropertyDefinition: "key",
  MethodDefinition: "key",
  AccessorProperty: "key",
  TSTypeAliasDeclaration: "id",
  TSInterfaceDeclaration: "id",
  TSEnumDeclaration: "id",
  TSEnumMember: "id",
  TSModuleDeclaration: "id",
  TSPropertySignature: "key",
  TSMethodSignature: "key",
  TSTypeParameter: "name",
  TSParameterProperty: "parameter",
  ImportSpecifier: "local",
  ImportDefaultSpecifier: "local",
  ImportNamespaceSpecifier: "local",
  CatchClause: "param",
  RestElement: "argument",
  AssignmentPattern: "left",
};

const functionTypes = new Set(["FunctionDeclaration", "FunctionExpression", "ArrowFunctionExpression", "TSDeclareFunction"]);

function namesSomething(node) {
  const parent = node.parent;
  if (!parent) return false;
  if (functionTypes.has(parent.type) && parent.params.includes(node)) return true;
  if (parent.type === "ArrayPattern") return true;
  if (parent.type === "Property") {
    // オブジェクトのリテラルでは鍵が名前で、分割代入では受け取る変数が名前である。
    if (parent.computed) return false;
    return parent.parent?.type === "ObjectPattern" ? parent.value === node : parent.key === node;
  }
  return parent[namingKeys[parent.type]] === node;
}

const idSpelling = {
  meta: { type: "suggestion", messages: { spelling: "`{{name}}` の末尾の `ID` を `Id` にする。" } },
  create(context) {
    return {
      Identifier(node) {
        if (uppercaseId.test(node.name) && namesSomething(node)) {
          context.report({ node, messageId: "spelling", data: { name: node.name } });
        }
      },
    };
  },
};

export default { meta: { name: "sshc" }, rules: { "id-spelling": idSpelling } };
