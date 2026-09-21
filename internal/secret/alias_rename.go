package secret

// AliasRename carries account authentication to the new name of the same peer.
// It is part of ConnectionSecretsMutation so config and vault commit together.
type AliasRename struct {
	From string
	To   string
}

func applyAliasRename(vault *Vault, rename AliasRename) (bool, error) {
	changed := false
	for _, kind := range []Kind{KindPassword, KindTOTP} {
		_, assigned := vault.SecretFor(kind, rename.From)
		changed = changed || assigned && rename.From != rename.To
		if err := vault.Rename(kind, rename.From, rename.To); err != nil {
			return false, err
		}
	}
	return changed, nil
}
