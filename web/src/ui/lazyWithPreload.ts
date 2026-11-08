import { lazy, type ComponentType, type LazyExoticComponent } from "react";

type ModuleWithDefault<Component> = { default: Component };

export type PreloadableComponent<Props extends object> = LazyExoticComponent<ComponentType<Props>> & {
  // preload fetches the component's code ahead of its first render. Calling it again does nothing more.
  preload: () => Promise<unknown>;
};

// alreadyLoaded hands React.lazy a module it already has. React.lazy waits for any promise, even a settled
// one, and draws the Suspense fallback for that frame; a thenable that calls back at once is read at once,
// so a page whose code was preloaded opens without its skeleton.
function alreadyLoaded<Component>(module: ModuleWithDefault<Component>): Promise<ModuleWithDefault<Component>> {
  const thenable = {
    then(onFulfilled: (value: ModuleWithDefault<Component>) => unknown) {
      onFulfilled(module);
      return thenable;
    },
  };
  return thenable as unknown as Promise<ModuleWithDefault<Component>>;
}

// lazyWithPreload is React.lazy with a way to fetch the code before the component is first drawn.
export function lazyWithPreload<Props extends object>(
  load: () => Promise<ModuleWithDefault<ComponentType<Props>>>,
): PreloadableComponent<Props> {
  let loaded: ModuleWithDefault<ComponentType<Props>> | null = null;
  let loading: Promise<ModuleWithDefault<ComponentType<Props>>> | null = null;
  const preload = () => {
    // A failed fetch is not kept: the first render then fetches again, as React.lazy alone would.
    loading ??= load().then(
      (module) => {
        loaded = module;
        return module;
      },
      (error: unknown) => {
        loading = null;
        throw error;
      },
    );
    return loading;
  };
  const component = lazy(() => (loaded === null ? preload() : alreadyLoaded(loaded)));
  return Object.assign(component, { preload });
}
