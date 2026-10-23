package buildcontract

// 所有者以外の起動・再実行がr540へ届かない条件を、CIと公開の契約で共有する。
const ownerLinuxRunner = "${{ (github.actor == github.repository_owner && github.triggering_actor == github.repository_owner) && 'r540-linux' || 'ubuntu-24.04' }}"

// macOS・Windows・ARM64の試験は元のmatrixのランナーを維持する。
const ownerLinuxMatrixRunner = "${{ (matrix.os == 'ubuntu-24.04' && github.actor == github.repository_owner && github.triggering_actor == github.repository_owner) && 'r540-linux' || matrix.os }}"
