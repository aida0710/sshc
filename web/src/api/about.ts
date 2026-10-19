import { apiClient } from "./client";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type About = components["schemas"]["About"];
export type AboutApi = { about(): Promise<About> };

export const aboutApi: AboutApi = {
  async about() {
    const about = await apiClient.read("/api/v1/about");
    return validateOpenAPISchema<About>("About", about);
  },
};
