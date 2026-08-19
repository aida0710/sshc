// Touch ID で守られた Keychain の項目を、3 つの操作にまとめる。
//
// **鍵は Keychain の中で作られ、Keychain の中で守られる。** ここが返すのは、預けた
// 秘密そのものか、断られたという事実だけである。生体のプロンプトを出すのは
// SecItemCopyMatching であって、この file ではない——アクセス制御を項目に付けて
// おけば、OS が読み出しの手前でそれを行う。
//
// **kSecAccessControlBiometryCurrentSet を選んでいる。** 指紋を足された時点で
// 預かりを無効にする、という意味である。登録した指と違う指で開いてはならない。
// 無効になったあとはパスワードの道へ戻るので、失うものは無い。

#import <Foundation/Foundation.h>
#import <LocalAuthentication/LocalAuthentication.h>
#import <Security/Security.h>

#include "biometric_darwin.h"

static NSString *const kService = @"com.github.aida0710.sshc";
static NSString *const kAccount = @"vault";

int sshc_biometric_available(void) {
	@autoreleasepool {
		LAContext *context = [[LAContext alloc] init];
		NSError *error = nil;
		BOOL ok = [context canEvaluatePolicy:LAPolicyDeviceOwnerAuthenticationWithBiometrics
		                               error:&error];
		return ok ? 1 : 0;
	}
}

// 同じ account の項目を先に消す。SecItemAdd は重複を errSecDuplicateItem で
// 断るので、預け直しがそこで止まらないようにする。
static void forget(void) {
	NSDictionary *query = @{
		(__bridge id)kSecClass: (__bridge id)kSecClassGenericPassword,
		(__bridge id)kSecAttrService: kService,
		(__bridge id)kSecAttrAccount: kAccount,
	};
	SecItemDelete((__bridge CFDictionaryRef)query);
}

int sshc_biometric_keep(const void *secret, int length) {
	@autoreleasepool {
		if (secret == NULL || length <= 0) return -1;
		CFErrorRef controlError = NULL;
		SecAccessControlRef control = SecAccessControlCreateWithFlags(
			kCFAllocatorDefault,
			kSecAttrAccessibleWhenUnlockedThisDeviceOnly,
			kSecAccessControlBiometryCurrentSet,
			&controlError);
		if (control == NULL) {
			if (controlError != NULL) CFRelease(controlError);
			return -1;
		}
		forget();
		NSData *data = [NSData dataWithBytes:secret length:(NSUInteger)length];
		NSDictionary *item = @{
			(__bridge id)kSecClass: (__bridge id)kSecClassGenericPassword,
			(__bridge id)kSecAttrService: kService,
			(__bridge id)kSecAttrAccount: kAccount,
			(__bridge id)kSecAttrAccessControl: (__bridge id)control,
			(__bridge id)kSecValueData: data,
		};
		OSStatus status = SecItemAdd((__bridge CFDictionaryRef)item, NULL);
		CFRelease(control);
		return (int)status;
	}
}

int sshc_biometric_reveal(void **out, int *length) {
	@autoreleasepool {
		if (out == NULL || length == NULL) return -1;
		*out = NULL;
		*length = 0;
		LAContext *context = [[LAContext alloc] init];
		// **PIN やパスワードへの読み替えを許さない。** 生体を求めた画面が、
		// 実際にはパスワードでも開けるのなら、それは別の機能である。
		context.localizedReason = @"Open the sshc vault";
		NSDictionary *query = @{
			(__bridge id)kSecClass: (__bridge id)kSecClassGenericPassword,
			(__bridge id)kSecAttrService: kService,
			(__bridge id)kSecAttrAccount: kAccount,
			(__bridge id)kSecReturnData: @YES,
			(__bridge id)kSecUseAuthenticationContext: context,
		};
		CFTypeRef result = NULL;
		OSStatus status = SecItemCopyMatching((__bridge CFDictionaryRef)query, &result);
		if (status != errSecSuccess) {
			if (result != NULL) CFRelease(result);
			return (int)status;
		}
		// **ARC は無い。** cgo が .m を訳すときは非 ARC なので、貰った参照は
		// こちらで返す。CoreFoundation のまま扱うのが、その約束が一番はっきり
		// する形である。
		CFDataRef data = (CFDataRef)result;
		CFIndex size = CFDataGetLength(data);
		void *copy = malloc((size_t)size);
		if (copy == NULL) {
			CFRelease(data);
			return -1;
		}
		memcpy(copy, CFDataGetBytePtr(data), (size_t)size);
		CFRelease(data);
		*out = copy;
		*length = (int)size;
		return 0;
	}
}

int sshc_biometric_forget(void) {
	@autoreleasepool {
		forget();
		return 0;
	}
}
