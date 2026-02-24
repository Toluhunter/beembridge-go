import { RiRadarFill } from "react-icons/ri";
import {
    StartPeerDiscovery,
    StopPeerDiscovery,
    GetDiscoveredPeers,
    ConnectToPeer,
    RespondToPeerConnectionRequest,
    DisconnectFromPeer
} from "../../../wailsjs/go/main/App.js"
import location from "../../assets/images/location-search_nesh.svg"
import { CircularProgress, MagnifyingGlass, ThreeCircles } from "react-loader-spinner";
import * as runtime from '../../../wailsjs/runtime/runtime.js';
import { MdDevices } from "react-icons/md";
import {
    useState,
    useEffect,
    useCallback
} from "react";


interface DiscoveryMessage {
    appId: string;       // A unique ID for your application (e.g., "MyFileTransferApp_v1")
    instanceId: string;  // A unique ID for this specific running instance
    peerName: string;    // A user-friendly name for this peer (e.g., computer name or user-defined)
    tcpPort: number;     // The TCP port this instance is listening on for file transfers
    timestamp: number;   // To help detect stale messages
}

// Define the structure of a discovered peer for internal tracking
export interface DiscoveredPeer extends DiscoveryMessage { // Exported for use in renderer process types
    lastSeen: string; // Timestamp when the last beacon from this peer was received
    ipAddress: string; // The IP address of the peer (from the UDP packet)
}


const PEER_DISCOVERY_TIME = 30 * 1000;

interface PeerViewProps {
    connectedPeers: DiscoveredPeer[];
    setConnectedPeers: React.Dispatch<React.SetStateAction<DiscoveredPeer[]>>;
}
export const PeerView: React.FC<PeerViewProps> = ({ connectedPeers, setConnectedPeers }) => {
    // State to manage the list of discovered peers
    const [discoveredPeers, setDiscoveredPeers] = useState<DiscoveredPeer[]>([]);
    // State to manage the list of connected peers
    // State to control the sonar animation and discovery mode
    const [incomingRequest, setIncomingRequest] = useState<{ peer: DiscoveredPeer, requestId: string, accept: () => void, reject: () => void } | null>(null);
    const [isDiscovering, setIsDiscovering] = useState(false);
    const [connectingPeerId, setConnectingPeerId] = useState<string | null>(null);
    const [isDiscoveryButtonDisabled, setIsDiscoveryButtonDisabled] = useState(false);
    // Animated ellipsis for discovery UI (".", "..", "...")
    const [ellipsis, setEllipsis] = useState<string>('');
    // Pagination state
    const [connectedPage, setConnectedPage] = useState(1);
    const [connectedPerPage, setConnectedPerPage] = useState<number>(3);
    const [discoveredPage, setDiscoveredPage] = useState(1);
    const [discoveredPerPage, setDiscoveredPerPage] = useState<number>(6);

    // Update per-page values to match Tailwind grid columns based on viewport width
    const updatePerPage = useCallback(() => {
        if (typeof window === 'undefined') return;
        const w = window.innerWidth;

        // Connected peers grid classes: grid-cols-1 sm:1 md:2 lg:3 xl:4 2xl:5
        let conn = 1;
        if (w >= 1536) conn = 5;
        else if (w >= 1280) conn = 4;
        else if (w >= 1024) conn = 3;
        else if (w >= 768) conn = 2;
        else conn = 1;

        // Discovered peers grid classes: grid-cols-1 sm:2 md:3 lg:4 xl:5 2xl:5
        let disc = 1;
        if (w >= 1536) disc = 5;
        else if (w >= 1280) disc = 5;
        else if (w >= 1024) disc = 4;
        else if (w >= 768) disc = 3;
        else if (w >= 640) disc = 2;
        else disc = 1;

        setConnectedPerPage(conn);
        setDiscoveredPerPage(disc);

        // Reset to first page if layout changes to avoid out-of-range pages
        setConnectedPage(1);
        setDiscoveredPage(1);
    }, []);

    useEffect(() => {
        updatePerPage();
        window.addEventListener('resize', updatePerPage);
        return () => window.removeEventListener('resize', updatePerPage);
    }, [updatePerPage]);

    // --- Dummy data for testing: 10 discovered and 10 connected peers ---
    const generateDummyPeer = (i: number, connected = false): DiscoveredPeer => ({
        appId: 'beembridge_dummy_app',
        instanceId: `${connected ? 'connected' : 'discovered'}-dummy-${i}`,
        peerName: `Dummy Peer ${i + 1}${connected ? ' (Connected)' : ''}`,
        tcpPort: 9000 + i,
        timestamp: Date.now(),
        lastSeen: new Date(Date.now() - i * 60000).toISOString(),
        ipAddress: `192.168.0.${10 + i}`
    });

    useEffect(() => {
        // Populate dummy lists for testing if empty
        // if (discoveredPeers.length === 0) {
        //     const dummyDiscovered = Array.from({ length: 10 }, (_v, i) => generateDummyPeer(i, false));
        //     setDiscoveredPeers(dummyDiscovered);
        // }

        // if (connectedPeers.length === 0) {
        //     const dummyConnected = Array.from({ length: 10 }, (_v, i) => generateDummyPeer(i, true));
        //     setConnectedPeers(dummyConnected);
        // }
    }, []);

    useEffect(() => {
        let intervalId: NodeJS.Timeout | undefined;

        if (isDiscovering) {
            intervalId = setInterval(() => {
                // NOTE: Commenting out real fetching for testing. Using dummy peers injected on mount.
                GetDiscoveredPeers().then(peers => {
                    const filteredDiscoveries = peers.filter(
                        newPeer => !connectedPeers.some(connectedPeer => connectedPeer.instanceId === newPeer.instanceId)
                    );
                    setDiscoveredPeers(filteredDiscoveries);
                });
            }, 1000); // Poll every 2 seconds
        }

        return () => {
            if (intervalId) {
                clearInterval(intervalId);
            }
        };
    }, [isDiscovering, connectedPeers]);

    // Animated ellipsis effect while discovering and no peers found
    useEffect(() => {
        let idx = 0;
        let timer: NodeJS.Timeout | undefined;

        if (isDiscovering && discoveredPeers.length === 0) {
            timer = setInterval(() => {
                idx = (idx + 1) % 4; // cycles 0..3
                setEllipsis('.'.repeat(idx));
            }, 500);
        } else {
            setEllipsis('');
        }

        return () => {
            if (timer) clearInterval(timer);
        };
    }, [isDiscovering, discoveredPeers.length]);

    const onConnectionResponse = useCallback((peer: DiscoveredPeer, status: string, reason?: string) => {
        console.log(`[RECEIVER] Connection status with ${peer.peerName}: ${status}${reason ? ` (${reason})` : ''}`);
        if (status === 'accepted') {
            // Move peer from discovered to connected list
            setConnectedPeers(prevConnected => {
                // Prevent adding duplicates to connectedPeers
                if (!prevConnected.some(p => p.instanceId === peer.instanceId)) {
                    return [...prevConnected, peer];
                }
                return prevConnected;
            });
            // Remove peer from discovered list as it's now connected
            setDiscoveredPeers(prevDiscovered => prevDiscovered.filter(p => p.instanceId !== peer.instanceId));
            console.log("[RECEIVER] Connection ready! You can send data now.");
        } else if (status === 'rejected' || status === 'failed' || status === 'timeout') {
            console.log("[RECEIVER] Connection failed or rejected. Please try another peer.");
            // If connection failed, ensure the peer is back in discovered if it was attempting to connect
            setDiscoveredPeers(prevDiscovered => {
                if (!prevDiscovered.some(p => p.instanceId === peer.instanceId)) {
                    return [...prevDiscovered, peer];
                }
                return prevDiscovered;
            });
        }
        setConnectingPeerId(null); // Always clear the connecting state after a response
    }, [setConnectedPeers]);

    useEffect(() => {
        // Wails event listeners
        const cleanupOnPeerConnectionRequest = runtime.EventsOn("onPeerConnectionRequest", (peer, requestId) => {
            console.log(`[RECEIVER] Connection request from ${peer.peerName} (${requestId})`);
            console.log(`[RECEIVER] Peer: ${JSON.stringify(peer)}`);
            const accept = () => {
                console.log(`[RECEIVER] Accepting connection request from ${peer.peerName}`);
                RespondToPeerConnectionRequest(requestId, true);
                setIncomingRequest(null);
            };

            const reject = (reason?: string) => {
                console.log(`[RECEIVER] Rejecting connection request from ${peer.peerName}${reason ? `: ${reason}` : ''}`);
                RespondToPeerConnectionRequest(requestId, false);
                setIncomingRequest(null);
            };
            setIncomingRequest({ peer, requestId, accept, reject });
        });

        const cleanupOnConnectionResponse = runtime.EventsOn("onConnectionResponse", (peer, status, reason) => {
            onConnectionResponse(peer, status, reason);
            setConnectingPeerId(null);
        });

        const cleanupOnPeerConnected = runtime.EventsOn("onPeerConnected", (peer) => {
            console.log(`[RECEIVER] Connection status with ${peer.peerName}: accepted`);
            onConnectionResponse(peer, 'accepted');
        });


        return () => {
            if (cleanupOnPeerConnectionRequest) cleanupOnPeerConnectionRequest();
            if (cleanupOnConnectionResponse) cleanupOnConnectionResponse();
            if (cleanupOnPeerConnected) cleanupOnPeerConnected();
        }
    }, [onConnectionResponse]); // Add connectedPeers to dependencies for accurate filtering

    /**
     * Initiates the peer discovery process.
     * Activates the sonar animation and calls the Electron API to start discovery.
     * Sets a timeout to stop the animation after a few seconds if no peers are found,
     * to prevent it from running indefinitely.
     */
    const startDiscovery = () => {
        if (isDiscoveryButtonDisabled) {
            return;
        }
        setIsDiscoveryButtonDisabled(true);
        setTimeout(() => setIsDiscoveryButtonDisabled(false), 1200);

        if (isDiscovering) {
            StopPeerDiscovery();
            setIsDiscovering(false);
            setDiscoveredPeers([]);
        } else {
            StartPeerDiscovery();
            setIsDiscovering(true);
        }
    };

    /**
     * Handles the action to connect to a discovered peer.
     * Moves the peer from the discovered list to the connected list.
     * @param peerToConnect The peer object to connect to.
     */
    const handleConnect = (peerToConnect: DiscoveredPeer) => {
        setConnectingPeerId(peerToConnect.instanceId); // Set connecting state
        console.log(`Attempting to connect to peer: ${peerToConnect.peerName} (${peerToConnect.instanceId})`);
        ConnectToPeer(peerToConnect);
    };

    // Disconnect handler: calls backend with peer ID and removes peer from connected list
    const handleDisconnect = async (peerToDisconnect: DiscoveredPeer) => {
        setConnectingPeerId(null);
        console.log(`Attempting to disconnect from peer: ${peerToDisconnect.peerName} (${peerToDisconnect.instanceId})`);
        try {
            await DisconnectFromPeer(peerToDisconnect.instanceId);
        } catch (err) {
            console.error("DisconnectFromPeer error:", err);
        } finally {
            setConnectedPeers(prev => prev.filter(p => p.instanceId !== peerToDisconnect.instanceId));
        }
    };

    return (
        <div className="flex flex-col h-full p-6">
            <div className="flex justify-between items-center mb-6">
                <h1 className="text-4xl font-bold text-white">Peers</h1>
                {incomingRequest && (
                    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black bg-opacity-50">
                        <div className="bg-gray-800 rounded-lg p-6 w-full max-w-md">
                            <h2 className="text-2xl font-bold text-white mb-4">Incoming Connection Request</h2>
                            <p className="text-gray-300 mb-4">
                                {incomingRequest.peer.peerName} ({incomingRequest.peer.ipAddress}:{incomingRequest.peer.tcpPort}) wants to connect.
                            </p>
                            <div className="flex justify-end">
                                <button
                                    className="mr-2 px-4 py-2 rounded-lg bg-red-500 text-white hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-opacity-50"
                                    onClick={() => {
                                        if (incomingRequest) {
                                            incomingRequest.reject();
                                            setIncomingRequest(null);
                                        }
                                    }}
                                >
                                    Reject
                                </button>
                                <button
                                    className="px-4 py-2 rounded-lg bg-green-500 text-white hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-green-500 focus:ring-opacity-50"
                                    onClick={() => {
                                        if (incomingRequest) {
                                            incomingRequest.accept();
                                            setConnectedPeers(prevConnected => {
                                                // Prevent adding duplicates to connectedPeers
                                                if (!prevConnected.some(p => p.instanceId === incomingRequest.peer.instanceId)) {
                                                    return [...prevConnected, incomingRequest.peer];
                                                }
                                                return prevConnected;
                                            });
                                            setIncomingRequest(null);
                                        }
                                    }}
                                >
                                    Accept
                                </button>
                            </div>
                        </div>
                    </div>
                )}
                {/* "Add Peer" button - moved below header (right-aligned) */}
            </div>

            {/* Discovery control: positioned under header, right-aligned */}
            <div className="flex justify-end mb-6">
                <button
                    className="modern-button text-white font-bold py-2 px-6 rounded-lg shadow-md"
                    onClick={startDiscovery}
                    disabled={isDiscoveryButtonDisabled}
                >
                    {isDiscovering ? 'Stop' : 'Find Peer'}
                </button>
            </div>
            {/* Connected Peers Section (hidden when no connected peers) */}
            {connectedPeers.length > 0 && (
                <div className="mb-6">
                    <div className="grid grid-cols-1 sm:grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 gap-6 justify-items-center">
                        {(() => {
                            const total = connectedPeers.length;
                            const totalPages = Math.ceil(total / connectedPerPage) || 1;
                            const start = (connectedPage - 1) * connectedPerPage;
                            const end = start + connectedPerPage;
                            const pageItems = connectedPeers.slice(start, end);
                            return (
                                <>
                                    {pageItems.map((peer) => (
                                        <div key={peer.instanceId} className="relative bg-gray-800/60 h-64 rounded-xl p-4 border border-green-700/40 max-sm:min-w-[265px] shadow-2xl w-full aspect-square flex flex-col justify-between break-words">
                                            <div className="space-y-1 flex flex-col items-center text-center">
                                                <MdDevices className="text-green-400 text-6xl mb-1" />
                                                <h3 className="text-lg font-semibold text-white leading-tight break-words">{peer.peerName} <span className="text-green-400 text-sm">(Connected)</span></h3>
                                                <p className="text-gray-400 text-sm break-words">ID: {peer.instanceId}</p>
                                            </div>

                                            <div className="flex flex-col items-center w-full justify-between pt-2 gap-2">
                                                <p className="text-gray-500 text-xs">Last active: {new Date(peer.lastSeen).toLocaleTimeString()}</p>
                                                <button
                                                    onClick={() => handleDisconnect(peer)}
                                                    className="border border-red-600 text-red-600 px-3 py-1 rounded-md hover:bg-red-600 hover:text-white transition-colors text-sm"
                                                >
                                                    Disconnect
                                                </button>
                                            </div>
                                        </div>
                                    ))}

                                    {totalPages > 1 && (
                                        <div className="col-span-full flex min-w-[285px] justify-between items-center p-2 mt-4 border-t border-gray-700 bg-gray-800/30 rounded">
                                            <div className="text-sm text-gray-400">
                                                Showing {Math.min(start + 1, total)} to {Math.min(end, total)} of {total} items
                                            </div>
                                            <div className="flex items-center space-x-2">
                                                <button
                                                    onClick={() => setConnectedPage(prev => Math.max(prev - 1, 1))}
                                                    disabled={connectedPage === 1}
                                                    className="px-2 py-1 text-gray-300 hover:text-white disabled:opacity-50 transition-colors"
                                                    aria-label="Previous Page"
                                                >
                                                    {'<'}
                                                </button>
                                                <div className="text-sm text-gray-300">{connectedPage} / {totalPages}</div>
                                                <button
                                                    onClick={() => setConnectedPage(prev => Math.min(prev + 1, totalPages))}
                                                    disabled={connectedPage === totalPages}
                                                    className="px-2 py-1 text-gray-300 hover:text-white disabled:opacity-50 transition-colors"
                                                    aria-label="Next Page"
                                                >
                                                    {'>'}
                                                </button>
                                            </div>
                                        </div>
                                    )}
                                </>
                            );
                        })()}
                    </div>
                </div>
            )}

            {/* Discovered Peers Section - fills remaining height */}
            <div className="flex-1 flex flex-col">
                {(connectedPeers.length == 0 || isDiscovering) && (

                    <div className="flex-1 overflow-y-auto rounded-2xl min-h-80 border border-gray-700 p-6 shadow-2xl relative">
                        {isDiscovering && discoveredPeers.length === 0 && (
                            <div className="absolute inset-0 z-40 flex items-center justify-center bg-black bg-opacity-30">
                                <div className="p-6 bg-transparent rounded flex flex-col items-center">
                                    <ThreeCircles
                                        visible={true}
                                        height="100"
                                        width="100"
                                        color="oklch(49.6% 0.265 301.924)"
                                        ariaLabel="three-circles-loading"
                                        wrapperStyle={{}}
                                        wrapperClass=""
                                    />
                                    <p className="text-white text-lg mt-4 text-center">Searching for peers
                                        <span className="ml-2">{ellipsis}</span>
                                    </p>
                                </div>
                            </div>
                        )}
                        {discoveredPeers.length === 0 && !isDiscovering ? (
                            <div className="flex flex-col items-center justify-center h-full text-center">
                                <div className="mb-8">
                                    <img src="/src/assets/images/location-search_nesh.svg" alt="Location Search" className="min-w-72 h-44" />

                                </div>
                                <p className="text-gray-400 text-xl mb-8">
                                    No peers detected yet.
                                </p>

                            </div>
                        ) : (
                            <div className="relative">
                                <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 2xl:grid-cols-5 gap-6 justify-items-center">
                                    {(() => {
                                        const total = discoveredPeers.length;
                                        const totalPages = Math.ceil(total / discoveredPerPage) || 1;
                                        const start = (discoveredPage - 1) * discoveredPerPage;
                                        const end = start + discoveredPerPage;
                                        const pageItems = discoveredPeers.slice(start, end);
                                        return (
                                            <>
                                                {pageItems.map((peer) => (
                                                    <div
                                                        key={peer.instanceId}
                                                        className={`bg-gray-800/60 rounded-xl p-4 border border-gray-700/40 shadow-2xl w-full h-80 max-sm:min-w-[175px] aspect-square flex flex-col justify-between break-words whitespace-normal
                                                        ${connectingPeerId === peer.instanceId ? 'connecting-animation' : ''}`}
                                                    >
                                                        <div className="flex justify-center">
                                                            <MdDevices className="text-blue-400 text-6xl" />
                                                        </div>
                                                        <div className="space-y-1">
                                                            <h3 className="text-lg font-semibold text-white leading-tight break-words whitespace-normal">{peer.peerName}</h3>
                                                            <p className="text-gray-400 text-sm break-words whitespace-normal">ID: {peer.instanceId}</p>
                                                            <p className="text-gray-500 text-xs break-words whitespace-normal">IP: {peer.ipAddress}</p>
                                                        </div>
                                                        <div className="flex flex-col gap-4 justify-between items-center pt-2">
                                                            <p className="text-gray-500 text-xs">Last: {new Date(peer.lastSeen).toLocaleTimeString()}</p>
                                                            <button
                                                                className="modern-button ml-4 py-1 px-3 text-white font-bold rounded-lg shadow-md text-sm"
                                                                onClick={() => handleConnect(peer)}
                                                                disabled={connectingPeerId === peer.instanceId} // Disable while connecting
                                                            >
                                                                {connectingPeerId === peer.instanceId ? (
                                                                    <span className="flex items-center justify-center">
                                                                        <svg className="animate-spin -ml-1 mr-2 h-4 w-4 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                                                                            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                                                                            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                                                                        </svg>
                                                                        Connecting...
                                                                    </span>
                                                                ) : (
                                                                    "Connect"
                                                                )}
                                                            </button>
                                                        </div>
                                                    </div>
                                                ))}

                                                {/* Loader moved to an overlay at the top of the discover container */}


                                                {/* Pagination for discovered peers */}
                                                {totalPages > 1 && (
                                                    <div className="col-span-full min-w-[290px] flex justify-between items-center p-2 mt-4 border-t border-gray-700 bg-gray-800/30 rounded">
                                                        <div className="text-sm text-gray-400">
                                                            Showing {Math.min(start + 1, total)} to {Math.min(end, total)} of {total} items
                                                        </div>
                                                        <div className="flex items-center space-x-2">
                                                            <button
                                                                onClick={() => setDiscoveredPage(prev => Math.max(prev - 1, 1))}
                                                                disabled={discoveredPage === 1}
                                                                className="px-2 py-1 text-gray-300 hover:text-white disabled:opacity-50 transition-colors"
                                                                aria-label="Previous Page"
                                                            >
                                                                {'<'}
                                                            </button>
                                                            <div className="text-sm text-gray-300">{discoveredPage} / {totalPages}</div>
                                                            <button
                                                                onClick={() => setDiscoveredPage(prev => Math.min(prev + 1, totalPages))}
                                                                disabled={discoveredPage === totalPages}
                                                                className="px-2 py-1 text-gray-300 hover:text-white disabled:opacity-50 transition-colors"
                                                                aria-label="Next Page"
                                                            >
                                                                {'>'}
                                                            </button>
                                                        </div>
                                                    </div>
                                                )}
                                            </>
                                        );
                                    })()}
                                </div>
                            </div>
                        )}
                    </div>
                )}
            </div>
        </div >
    );
};