'use client';
import './App.css'; // Import global styles
import React, { useEffect, useState } from 'react'; // Import useRef for file input
import logoBb from '../public/logo-bb.svg';
import { PeerView } from './components/views/peers.js';
import { DiscoveredPeer } from './components/views/peers.js';
import { ExplorerView, SelectedItem as SelectedFile } from './components/views/explorer.js';
import { TransferHistoryView } from './components/views/transfer-history.js';
import { ActiveTransferView, ActiveTransferDisplayItem } from './components/views/active-transfers.js';
import { SettingsView } from './components/views/settings.js';
import { SlPeople } from "react-icons/sl";
import { BiTransfer } from "react-icons/bi";
import { LuFolders } from "react-icons/lu";
import { FaHistory } from "react-icons/fa";
import { CiSettings } from "react-icons/ci";
import { TbLayoutSidebarLeftCollapseFilled as CollapseIcon } from "react-icons/tb";
import { TbLayoutSidebarRightCollapseFilled } from "react-icons/tb";
import { IconType } from 'react-icons';
import * as runtime from '../wailsjs/runtime/runtime.js';
import { InitiateFileTransfer } from '../wailsjs/go/main/App.js';
import { main } from '../wailsjs/go/models.js';

// Define an interface for a Peer object (example)
// Define an interface for the shape of a sidebar item
interface SidebarItem {
    id: 'peers' | 'history' | 'active-transfers' | 'explorer' | 'settings'; // Added 'active-transfers'
    name: string;
    icon: IconType; // Updated to use IconType
}


// Sidebar Items data
const sidebarItems: SidebarItem[] = [
    { id: 'peers', name: 'Peers', icon: SlPeople },
    { id: 'history', name: 'Transfer History', icon: FaHistory },
    { id: 'active-transfers', name: 'Active Transfers', icon: BiTransfer }, // New item for active transfers
    { id: 'explorer', name: 'Explorer', icon: LuFolders },
    { id: 'settings', name: 'Settings', icon: CiSettings }, // New settings item
];

// Main App Component
const App = () => {
    const [activeView, setActiveView] = useState<SidebarItem['id']>('peers');
    const [isSidebarCollapsed, setIsSidebarCollapsed] = useState<boolean>(false); // State for sidebar collapse
    const [isMouseOverSidebar, setIsMouseOverSidebar] = useState<boolean>(false);
    const [selectedFiles, setSelectedFiles] = useState<SelectedFile[]>([]);
    const [connectedPeers, setConnectedPeers] = useState<DiscoveredPeer[]>([]);
    const [userName, setUserName] = useState("BeemBridge User"); // Made userName mutable
    const [userId, setUserId] = useState("BB_USER_1234567890"); // Made userId mutable
    const [storagePath, setStoragePath] = useState<string>("");
    const [activeTransfers, setActiveTransfers] = useState<ActiveTransferDisplayItem[]>([]);
    const [hashingProgress, setHashingProgress] = useState<{ [key: string]: number }>({});

    const handleAddSelectedFiles = (newFiles: SelectedFile[]) => {
        const uniqueNewFiles = newFiles.filter(newFile =>
            !selectedFiles.some(existingFile =>
                existingFile.name === newFile.name && existingFile.size === newFile.size
            )
        );
        setSelectedFiles(prevFiles => [...prevFiles, ...uniqueNewFiles]);
    };

    const handleRemoveSelectedFile = (fileToRemove: SelectedFile) => {
        setSelectedFiles(prevFiles =>
            prevFiles.filter(file =>
                !(file.name === fileToRemove.name && file.size === fileToRemove.size)
            )
        );
    };

    const handleSendFilesToPeers = (files: SelectedFile[], targetPeers: DiscoveredPeer[]) => {
        if (targetPeers.length > 0) {
            const peerID = targetPeers[0].instanceId;
            const itemsToSend = files.map(f => main.SelectedItem.createFrom(f));
            InitiateFileTransfer(peerID, itemsToSend);
        }
        setActiveView('active-transfers');
        setSelectedFiles([]); // Clear selected files after sending
    };

    const handleUpdateUserNameInMainProcess = async (newName: string) => {
        // TODO: Implement with Wails
        return false;
    };

    const handleGenerateNewUserIdInMainProcess = async () => {
        // TODO: Implement with Wails
    };

    // function to handle setting of storage path
    const handleSetStoragePath = async () => {
        // TODO: Implement with Wails
    };

    const toggleSidebar = () => {
        setIsSidebarCollapsed(!isSidebarCollapsed);
    };

    useEffect(() => {
        runtime.EventsOn("onProgressUpdate", (progress) => {
            setActiveTransfers(prevTransfers => {
                const existingIndex = prevTransfers.findIndex(t => t.fileId === progress.fileId);

                let derivedStatus: ActiveTransferDisplayItem['status'] = 'in-progress';
                if (progress.percentage >= 100) {
                    derivedStatus = 'completed';
                } else if (progress.percentage < 0) { // Assuming negative indicates failure
                    derivedStatus = 'failed';
                } else if (progress.percentage === 0 && progress.transferredBytes === 0) {
                    derivedStatus = 'pending';
                }

                const updatedDisplayItem: ActiveTransferDisplayItem = { ...progress, status: derivedStatus };

                if (existingIndex > -1) {
                    const updatedTransfers = [...prevTransfers];
                    updatedTransfers[existingIndex] = updatedDisplayItem;
                    return updatedTransfers;
                } else {
                    return [...prevTransfers, updatedDisplayItem];
                }
            });
        });

        runtime.EventsOn("onHashingProgress", (progress) => {
            if (progress.percentage === 100) {
                setHashingProgress(prev => {
                    const newProgress = { ...prev };
                    delete newProgress[progress.filePath];
                    return newProgress;
                });
            } else {
                setHashingProgress(prev => ({ ...prev, [progress.filePath]: progress.percentage }));
            }
        });

        runtime.EventsOn("onTransferComplete", (result) => {
            setActiveTransfers(prevTransfers => {
                const existingIndex = prevTransfers.findIndex(t => t.fileId === result.fileId);
                if (existingIndex > -1) {
                    const updatedTransfers = [...prevTransfers];
                    updatedTransfers[existingIndex].status = result.status;
                    if (result.status === 'completed') updatedTransfers[existingIndex].percentage = 100;
                    return updatedTransfers;
                }
                return prevTransfers; // Or add if not present
            });
        });

    }, []); // Empty dependency array to run only once on mount

    // Function to simulate opening file explorer
    return (
        <div className="flex h-screen w-screen bg-gray-950 text-white font-inter overflow-hidden">
            {/* Global styles for Inter font and modern aesthetics */}
            {/* Sidebar */}
            <aside
                onMouseEnter={() => setIsMouseOverSidebar(true)}
                onMouseLeave={() => setIsMouseOverSidebar(false)}
                className={`flex flex-col border-r border-gray-800 py-6 transition-all duration-300 ease-in-out relative
          ${isSidebarCollapsed ? 'w-20 items-center' : 'w-1/5 min-w-[220px] max-w-[280px]'}`
                }
                style={{ boxShadow: '2px 0 10px rgba(0,0,0,0.3)' }} /* Subtle shadow for depth */
            >
                <div className="flex-grow flex flex-col w-full">
                    {/* Logo/App Name */}
                    <div className={`mb-8 flex items-center ${isSidebarCollapsed ? 'justify-center h-8' : 'justify-between'}`}>
                        {isSidebarCollapsed ? (
                            isMouseOverSidebar ? (
                                <button
                                    onClick={toggleSidebar}
                                    className="p-2 rounded-lg shadow-lg text-gray-300 hover:bg-gray-700 focus:outline-none transition-transform duration-300 z-10 bg-gray-800"
                                    aria-label="Toggle Sidebar"
                                >
                                    <TbLayoutSidebarRightCollapseFilled />
                                </button>
                            ) : (
                                <img src={logoBb} alt="BB Logo" className="pr-2 h-10 w-auto" />
                            )
                        ) : (
                            <>
                                <div className="flex items-center">
                                    <img src={logoBb} alt="BeemBridge Logo" className="h-8 w-auto mx-2" />
                                    <h2 className="text-2xl font-extrabold text-white">BeemBridge</h2>
                                </div>
                                <button
                                    onClick={toggleSidebar}
                                    className="p-2 rounded-lg shadow-lg text-gray-300 hover:bg-gray-700 focus:outline-none transition-transform duration-300 z-10 mr-2"
                                    aria-label="Toggle Sidebar"
                                >
                                    <CollapseIcon size={20} />
                                </button>
                            </>
                        )}
                    </div>

                    {/* Navigation */}
                    <nav className="mt-8">
                        <ul>
                            {sidebarItems.map((item) => {
                                const Icon = item.icon;
                                return (
                                    <li key={item.id} className="group relative">
                                        <button
                                            onClick={() => setActiveView(item.id)}
                                            className={`flex items-center w-full px-4 py-3 my-5 rounded-xl text-left transition-colors duration-200
                                        ${activeView === item.id
                                                    ? 'bg-purple-700 text-white'
                                                    : 'text-gray-300 hover:bg-gray-700 hover:text-white'
                                                }
                                        ${isSidebarCollapsed ? 'justify-center px-2' : ''}`
                                            }
                                        >
                                            <span className={`text-2xl mr-3
                                                ${activeView === item.id ? 'text-white' : 'text-gray-400 group-hover:text-white'}`
                                            }><Icon /></span>
                                            {!isSidebarCollapsed && (
                                                <span className="font-medium text-lg ml-3">{item.name}</span>
                                            )}
                                        </button>
                                        {isSidebarCollapsed && (
                                            <span className="absolute left-full ml-2 w-auto p-2 min-w-max rounded-md shadow-md text-white bg-gray-800 text-xs font-bold transition-all duration-100 scale-0 group-hover:scale-100 origin-left">
                                                {item.name}
                                            </span>
                                        )}
                                    </li>
                                );
                            })}
                        </ul>
                    </nav>
                </div>

                {/* User Profile Info */}
                <div className={`mt-auto pt-6 border-t border-gray-700 ${isSidebarCollapsed ? 'flex flex-col items-center' : ''}`}>
                    <div className={`flex items-center ${isSidebarCollapsed ? 'flex-col' : ''}`}>
                        {/* Avatar Placeholder */}
                        <div className={`w-12 h-12 bg-blue-600 rounded-full flex items-center justify-center text-white text-xl font-bold ${isSidebarCollapsed ? 'mb-2' : 'mr-3'}`}>
                            {userName.charAt(0).toUpperCase()}
                        </div>
                        {!isSidebarCollapsed && (
                            <div>
                                <p className="text-white font-semibold">{userName}</p>
                                <p className="text-gray-400 text-sm break-all">ID: {userId}</p>
                            </div>
                        )}
                        {isSidebarCollapsed && (
                            <p className="text-gray-400 text-xs text-center break-all mt-1">{userId.substring(0, 5)}...</p>
                        )}
                    </div>
                </div>
            </aside>

            {/* Main Content */}
            <main className="flex-1 p-8 overflow-auto">
                {activeView === 'peers' && (
                    <PeerView
                        connectedPeers={connectedPeers}
                        setConnectedPeers={setConnectedPeers}
                    />
                )}

                {activeView === 'history' && (
                    <TransferHistoryView />
                )}

                {activeView === 'active-transfers' && ( // New view for Active Transfers
                    <ActiveTransferView
                        activeTransfers={activeTransfers}
                        hashingProgress={hashingProgress}
                    />
                )}

                {activeView === 'explorer' && (
                    <ExplorerView
                        selectedFiles={selectedFiles}
                        onAddFiles={handleAddSelectedFiles}
                        onRemoveFile={handleRemoveSelectedFile}
                        connectedPeers={connectedPeers} // Pass connected peers
                        onSendFilesToPeers={handleSendFilesToPeers} // Pass send files handler
                    />
                )}

                {activeView === 'settings' && (
                    <SettingsView
                        currentUserName={userName}
                        currentUserId={userId}
                        storagePath={storagePath}
                        onUpdateUserNameInMainProcess={handleUpdateUserNameInMainProcess}
                        onGenerateNewUserId={handleGenerateNewUserIdInMainProcess}
                        onSetStoragePath={handleSetStoragePath} // Pass the storage path handler`
                    />
                )}
            </main>
        </div>
    );
};

export default App;
