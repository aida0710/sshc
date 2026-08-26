package com.github.aida0710.sshc;

/** Android frameworkに依存しない、issue共有用の診断文生成。 */
final class FailureReport {
    private FailureReport() {}

    static String render(
            String version,
            String code,
            String detail,
            String androidRelease,
            int androidSdk,
            String manufacturer,
            String model,
            String abis) {
        return "Version: " + available(version)
                + "\nCode: " + available(code)
                + "\nDetail: " + available(detail)
                + "\nAndroid: " + available(androidRelease) + " (SDK " + androidSdk + ")"
                + "\nDevice: " + device(manufacturer, model)
                + "\nABI: " + available(abis);
    }

    private static String available(String value) {
        if (value == null || value.isBlank()) return "Unavailable";
        return value;
    }

    private static String device(String manufacturer, String model) {
        String maker = available(manufacturer);
        String product = available(model);
        if (product.regionMatches(true, 0, maker, 0, maker.length())) return product;
        return maker + " " + product;
    }
}
