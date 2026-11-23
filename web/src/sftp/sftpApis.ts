import { chmodApi, createChmodApi, type ChmodApi } from "./chmodApi";
import { createSftpApi, sftpApi, type SftpApi } from "./api";
import { createLocalMutationApi, localMutationApi, type LocalMutationApi } from "./localMutationApi";
import { createRemoteMetadataApi, remoteMetadataApi, type RemoteMetadataApi } from "./remoteMetadataApi";

// SftpApis は、ある sshc エンジンのファイルの操作の API である。「ローカル」は、そのエンジンのマシンのファイルを指す。
export type SftpApis = {
  files: SftpApi;
  chmod: ChmodApi;
  localMutation: LocalMutationApi;
  remoteMetadata: RemoteMetadataApi;
};

export const thisMachineSftpApis: SftpApis = { files: sftpApi, chmod: chmodApi, localMutation: localMutationApi, remoteMetadata: remoteMetadataApi };

// createSftpApis は、base（engineApiBase）にあるエンジンの SftpApis を作る。
export function createSftpApis(base: string): SftpApis {
  return {
    files: createSftpApi(base), chmod: createChmodApi(base),
    localMutation: createLocalMutationApi(base), remoteMetadata: createRemoteMetadataApi(base),
  };
}
