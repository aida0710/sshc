import { apiClient } from "./client";
import { localApiBase } from "./remoteEngines";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type About = components["schemas"]["About"];
export type AboutApi = { about(): Promise<About> };

// createAboutApi は、base（engineApiBase）のエンジンの sshc のバージョンなどを読む API である。
export function createAboutApi(base: string): AboutApi {
  return {
    async about() {
      const about = await apiClient.read(`${base}/about`);
      return validateOpenAPISchema<About>("About", about);
    },
  };
}

export const aboutApi = createAboutApi(localApiBase);
