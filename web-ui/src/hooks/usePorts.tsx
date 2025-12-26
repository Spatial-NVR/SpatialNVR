// React hook and context for service port discovery
import { createContext, useContext, useEffect, useState, ReactNode } from 'react';
import { PortConfig, fetchPorts, getPorts, getGo2RTCUrl, getGo2RTCWsUrl, getApiUrl } from '../lib/ports';

interface PortsContextValue {
  ports: PortConfig;
  isLoaded: boolean;
  go2rtcUrl: string;
  go2rtcWsUrl: string;
  apiUrl: string;
}

const PortsContext = createContext<PortsContextValue | null>(null);

interface PortsProviderProps {
  children: ReactNode;
}

/**
 * Provider that fetches and provides port configuration to the app
 * Wrap your app with this provider to enable port discovery
 */
export function PortsProvider({ children }: PortsProviderProps) {
  const [isLoaded, setIsLoaded] = useState(false);
  const [ports, setPorts] = useState<PortConfig>(getPorts());

  useEffect(() => {
    let mounted = true;

    fetchPorts().then((loadedPorts) => {
      if (mounted) {
        setPorts(loadedPorts);
        setIsLoaded(true);
      }
    });

    return () => {
      mounted = false;
    };
  }, []);

  const value: PortsContextValue = {
    ports,
    isLoaded,
    go2rtcUrl: getGo2RTCUrl(),
    go2rtcWsUrl: getGo2RTCWsUrl(),
    apiUrl: getApiUrl(),
  };

  return (
    <PortsContext.Provider value={value}>
      {children}
    </PortsContext.Provider>
  );
}

/**
 * Hook to access port configuration
 * Must be used within a PortsProvider
 */
export function usePorts(): PortsContextValue {
  const context = useContext(PortsContext);
  if (!context) {
    // If used outside provider, return defaults
    return {
      ports: getPorts(),
      isLoaded: false,
      go2rtcUrl: getGo2RTCUrl(),
      go2rtcWsUrl: getGo2RTCWsUrl(),
      apiUrl: getApiUrl(),
    };
  }
  return context;
}

// Re-export the synchronous getters for use outside React
export { getPorts, getGo2RTCUrl, getGo2RTCWsUrl, getApiUrl } from '../lib/ports';
