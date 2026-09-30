//go:build windows

package windowsacl

// RestrictFile は、末尾の reparse point を追わずにパスを開き、そのハンドルで
// ポリシーを適用する。
//
// 製品はパスを開き直さず、手元のハンドルへ RestrictFileHandle を使う。これは
// os.WriteFile などで作ったファイルへ、テストが同じポリシーを当てるための入口である。
func RestrictFile(path string) error {
	if err := ValidatePrivatePath(path); err != nil {
		return err
	}
	file, err := openObjectToRestrict(path, fileObject)
	if err != nil {
		return err
	}
	defer file.Close()
	return restrictFileHandle(file)
}
