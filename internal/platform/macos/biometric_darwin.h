// Go 側から見た、Touch ID で守られた Keychain の輪郭。
#ifndef SSHC_BIOMETRIC_DARWIN_H
#define SSHC_BIOMETRIC_DARWIN_H

int sshc_biometric_available(void);
int sshc_biometric_keep(const void *secret, int length);
int sshc_biometric_reveal(void **out, int *length);
int sshc_biometric_forget(void);

#endif
