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
import { Sidebar, SidebarItem } from './components/shared/sidebar.js';
import * as runtime from '../wailsjs/runtime/runtime.js';
import { InitiateFileTransfer } from '../wailsjs/go/main/App.js';
import { main } from '../wailsjs/go/models.js';
import { Footer } from './components/shared/footer.js';

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

    const handleRemoveSelectedFiles = (filesToRemove: SelectedFile[]) => {
        setSelectedFiles(prevFiles =>
            prevFiles.filter(existingFile =>
                !filesToRemove.some(fileToRemove =>
                    existingFile.name === fileToRemove.name && existingFile.size === fileToRemove.size
                )
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
        <div className="flex flex-col h-screen w-screen bg-gray-950 text-white font-inter overflow-hidden">
            <div className="flex flex-1 overflow-y-auto">
                {/* Global styles for Inter font and modern aesthetics */}
                {/* Sidebar */}
                <Sidebar
                    logoBb={logoBb}
                    isSidebarCollapsed={isSidebarCollapsed}
                    isMouseOverSidebar={isMouseOverSidebar}
                    setIsMouseOverSidebar={setIsMouseOverSidebar}
                    toggleSidebar={toggleSidebar}
                    activeView={activeView}
                    setActiveView={setActiveView}
                    userName={userName}
                    userId={userId}
                />

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
                            onRemoveFiles={handleRemoveSelectedFiles}
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
            <Footer />
        </div>
    );
};

export default App;
