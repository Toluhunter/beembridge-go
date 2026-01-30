import React, { useState } from "react";
import { DiscoveredPeer } from "./peers.js";
import { GetFileStats, OpenDirectoryDialog, OpenFileDialog } from "../../../wailsjs/go/main/App.js";


// Define an interface for a selected file to include properties we care about
export interface SelectedItem {
    path: string;
    name: string;
    size: number;
    isDirectory: boolean;
    lastModified: string;
}

interface ExplorerViewProps {
    selectedFiles: SelectedItem[];
    onAddFiles: (files: SelectedItem[]) => void;
    onRemoveFiles: (filesToRemove: SelectedItem[]) => void; // Updated to accept an array
    connectedPeers: DiscoveredPeer[]; // Added connectedPeers prop
    onSendFilesToPeers: (files: SelectedItem[], targetPeers: DiscoveredPeer[]) => void; // New prop for sending files
}

export const ExplorerView: React.FC<ExplorerViewProps> = ({ selectedFiles, onAddFiles, onRemoveFiles, connectedPeers, onSendFilesToPeers }) => {
    const [showSendModal, setShowSendModal] = useState(false);
    const [selectedPeerForSending, setSelectedPeerForSending] = useState<DiscoveredPeer | null>(null);
    const [showAddOptions, setShowAddOptions] = useState(false);
    const [selectedItemsForRemoval, setSelectedItemsForRemoval] = useState<Set<string>>(new Set()); // New state for checkboxes
    const [sortColumn, setSortColumn] = useState<keyof SelectedItem | null>('name');
    const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('asc');

    const handleOpenFile = async () => {
        try {
            const filePaths = await OpenFileDialog();
            if (filePaths && filePaths.length > 0) {
                const files = await GetFileStats(filePaths);
                onAddFiles(files);
            }
        } catch (error) {
            console.error("Error opening files:", error);
        }
    };

    const handleAddFilesClick = () => {
        handleOpenFile();
    };

    const formatFileSize = (bytes: number): string => {
        if (bytes === 0) return '0 Bytes';
        const k = 1024;
        const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    };

    const handleSendClick = () => {
        setShowSendModal(true);
        // Reset selected peers for sending when opening the modal
        setSelectedPeerForSending(null);
    };

    const handleCloseSendModal = () => {
        setShowSendModal(false);
        setSelectedPeerForSending(null); // Clear selected peer when closing
    };

    const handlePeerSelectionChange = (peer: DiscoveredPeer) => {
        setSelectedPeerForSending(peer);
    };

    const handleConfirmSend = () => {
        if (selectedPeerForSending && selectedFiles.length > 0) {
            selectedFiles.forEach(file => console.log("Selected File:", file.name));
            handleCloseSendModal(); // Close modal after initiating send
            onSendFilesToPeers(selectedFiles, [selectedPeerForSending]); // using list because program will send to multiple peers in the future
        } else {
            console.warn("No files selected or no peer chosen for sending.");
        }
    };

    const handleAddDirectoryClick = async () => {
        try {
            const dirPath = await OpenDirectoryDialog();
            if (dirPath) {
                const files = await GetFileStats([dirPath]);
                onAddFiles(files);
            }
        } catch (error) {
            console.error("Error opening directory:", error);
        }
    };

    const handleCheckboxChange = (path: string, isChecked: boolean) => {
        setSelectedItemsForRemoval(prev => {
            const newSet = new Set(prev);
            if (isChecked) {
                newSet.add(path);
            } else {
                newSet.delete(path);
            }
            return newSet;
        });
    };

    const handleRemoveSelected = () => {
        const itemsToRemove = selectedFiles.filter(file => selectedItemsForRemoval.has(file.path));
        onRemoveFiles(itemsToRemove); // Call with array
        setSelectedItemsForRemoval(new Set()); // Clear selection after removal
    };

    const getType = (file: SelectedItem): string => {
        if (file.isDirectory) return 'Folder';
        const parts = file.name.split('.');
        if (parts.length > 1) {
            return parts[parts.length - 1].toUpperCase(); // Return extension in uppercase
        }
        return 'File';
    };

    const handleSort = (column: keyof SelectedItem | 'isDirectory') => {
        if (sortColumn === column) {
            setSortDirection(prev => (prev === 'asc' ? 'desc' : 'asc'));
        } else {
            setSortColumn(column);
            setSortDirection('asc');
        }
    };

    const sortedFiles = [...selectedFiles].sort((a, b) => {
        if (!sortColumn) return 0;

        let aValue: any;
        let bValue: any;

        if (sortColumn === 'isDirectory') {
            aValue = getType(a);
            bValue = getType(b);
        } else {
            aValue = a[sortColumn];
            bValue = b[sortColumn];
        }


        // Handle boolean sorting for isDirectory
        if (sortColumn === 'isDirectory') {
            if (aValue === bValue) return 0;
            if (sortDirection === 'asc') {
                return aValue ? -1 : 1; // true (directories) come first
            } else {
                return aValue ? 1 : -1; // false (files) come first
            }
        }

        // Handle string and number sorting
        if (typeof aValue === 'string' && typeof bValue === 'string') {
            return sortDirection === 'asc' ? aValue.localeCompare(bValue) : bValue.localeCompare(aValue);
        } else if (typeof aValue === 'number' && typeof bValue === 'number') {
            return sortDirection === 'asc' ? aValue - bValue : bValue - aValue;
        }

        return 0; // Should not happen if types are consistent
    });


    return (
        <div className="relative flex flex-col h-full p-8 shadow-lg overflow-y-auto">
            <h1 className="text-4xl font-bold text-white mb-4 text-start">File Explorer</h1>
            <p className="text-gray-400 text-lg mb-6 text-start">Browse and manage your files for transfer.</p>

            <button
                onChange={handleOpenFile}
                className="hidden"
            />

            <div className=" p-4 py-10 overflow-y-auto custom-scrollbar">
                <div className="rounded-lg flex flex-col items-center justify-center h-full text-gray-500 p-10">
                    <div className="border border-dashed bg-gray-800 w-100 rounded-lg flex flex-col items-center justify-center h-full text-gray-500 p-10">
                        <svg className="w-16 h-16 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M9 13h6m-3-3v6m5 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"></path>
                        </svg>
                        <p className="text-lg">No files selected yet.</p>
                        <p className="text-sm">Click the `+`` button to add files.</p>
                    </div>
                </div>

                <div className="bg-gray-800 rounded-lg shadow-md overflow-hidden">
                    <table className="min-w-full divide-y divide-gray-700">
                        <thead className="bg-gray-700">
                            <tr>
                                <th className="px-4 py-3 text-left text-xs font-medium text-gray-300 uppercase tracking-wider">
                                    <input
                                        type="checkbox"
                                        className="form-checkbox h-4 w-4 text-blue-600 transition duration-150 ease-in-out"
                                        checked={selectedItemsForRemoval.size === selectedFiles.length && selectedFiles.length > 0}
                                        onChange={(e) => {
                                            if (e.target.checked) {
                                                setSelectedItemsForRemoval(new Set(selectedFiles.map(file => file.path)));
                                            } else {
                                                setSelectedItemsForRemoval(new Set());
                                            }
                                        }}
                                    />
                                </th>
                                <th
                                    className="px-6 py-3 text-left text-xs font-medium text-gray-300 uppercase tracking-wider cursor-pointer"
                                    onClick={() => handleSort('name')}
                                >
                                    Name
                                    {sortColumn === 'name' && (sortDirection === 'asc' ? ' 🔼' : ' 🔽')}
                                </th>
                                <th
                                    className="px-6 py-3 text-left text-xs font-medium text-gray-300 uppercase tracking-wider cursor-pointer"
                                    onClick={() => handleSort('isDirectory')}
                                >
                                    Is Folder
                                    {sortColumn === 'isDirectory' && (sortDirection === 'asc' ? ' 🔼' : ' 🔽')}
                                </th>
                                <th
                                    className="px-6 py-3 text-left text-xs font-medium text-gray-300 uppercase tracking-wider cursor-pointer"
                                    onClick={() => handleSort('isDirectory')}
                                >
                                    Type
                                    {sortColumn === 'isDirectory' && (sortDirection === 'asc' ? ' 🔼' : ' 🔽')}
                                </th>
                                <th
                                    className="px-6 py-3 text-left text-xs font-medium text-gray-300 uppercase tracking-wider cursor-pointer"
                                    onClick={() => handleSort('size')}
                                >
                                    Size
                                    {sortColumn === 'size' && (sortDirection === 'asc' ? ' 🔼' : ' 🔽')}
                                </th>
                                <th className="px-6 py-3 text-left text-xs font-medium text-gray-300 uppercase tracking-wider">
                                    Actions
                                </th>
                            </tr>
                        </thead>
                        <tbody className="divide-y divide-gray-700">
                            {sortedFiles.map((file) => (
                                <tr key={file.path} className="hover:bg-gray-700">
                                    <td className="px-4 py-4 whitespace-nowrap">
                                        <input
                                            type="checkbox"
                                            className="form-checkbox h-4 w-4 text-blue-600 transition duration-150 ease-in-out"
                                            checked={selectedItemsForRemoval.has(file.path)}
                                            onChange={(e) => handleCheckboxChange(file.path, e.target.checked)}
                                        />
                                    </td>
                                    <td className="px-6 py-4 whitespace-nowrap text-sm font-medium text-white flex items-center">
                                        <span className="mr-2 text-blue-400">
                                            {file.isDirectory ? '📁' : '📄'}
                                        </span>
                                        {file.name}
                                    </td>
                                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-400">
                                        {file.isDirectory ? 'Yes' : 'No'}
                                    </td>
                                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-400">
                                        {getType(file)}
                                    </td>
                                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-400">
                                        {formatFileSize(file.size)}
                                    </td>
                                    <td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
                                        <button
                                            onClick={() => onRemoveFiles([file])} // Call with single-item array
                                            className="text-red-600 hover:text-red-900 ml-2"
                                            aria-label={`Remove ${file.name}`}
                                        >
                                            Remove
                                        </button>
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                    {selectedItemsForRemoval.size > 0 && (
                        <div className="p-4 bg-gray-700 border-t border-gray-600 flex justify-end">
                            <button
                                onClick={handleRemoveSelected}
                                className="px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-lg shadow-md transition-colors duration-150"
                            >
                                Remove Selected ({selectedItemsForRemoval.size})
                            </button>
                        </div>
                    )}
                </div>

            </div>

            {/* Send Button - Always visible now */}
            <button
                onClick={handleSendClick}
                className="modern-button absolute bottom-6 left-6 py-3 px-6 rounded-lg text-lg font-bold shadow-lg transition-all duration-200 focus:outline-none focus:ring-4 focus:ring-blue-500 focus:ring-opacity-50"
                aria-label="Send Files"
                disabled={selectedFiles.length === 0} // Disable if no files are selected
            >
                Send Selected Files ({selectedFiles.length})
            </button>

            {/* Plus SVG Circle Icon for adding files, now with animated options */}
            <div className="absolute bottom-6 right-6 flex flex-col items-end z-20">
                {/* Animated Buttons */}
                <div
                    className={`flex flex-col items-end mb-2 transition-all duration-300 ${showAddOptions ? "opacity-100 translate-y-0 pointer-events-auto" : "opacity-0 translate-y-4 pointer-events-none"
                        }`}
                >
                    <button
                        onClick={handleAddFilesClick}
                        className="mb-2 w-48 py-2 px-4 bg-blue-500 hover:bg-blue-600 rounded-lg text-white font-semibold shadow-lg transition-all duration-200"
                        style={{ transitionDelay: showAddOptions ? "50ms" : "0ms" }}
                    >
                        Add Files
                    </button>
                    <button
                        onClick={handleAddDirectoryClick}
                        className="w-48 py-2 px-4 bg-green-500 hover:bg-green-600 rounded-lg text-white font-semibold shadow-lg transition-all duration-200"
                        style={{ transitionDelay: showAddOptions ? "100ms" : "0ms" }}
                    >
                        Add Directory
                    </button>
                </div>
                {/* Main + Button */}
                <button
                    onClick={() => setShowAddOptions((prev) => !prev)}
                    className={`w-16 h-16 bg-blue-600 hover:bg-blue-700 rounded-full flex items-center justify-center text-white text-5xl font-light shadow-lg transition-all duration-200 focus:outline-none focus:ring-4 focus:ring-blue-500 focus:ring-opacity-50 ${showAddOptions ? "rotate-45" : ""
                        }`}
                    aria-label="Add Files"
                    style={{ transition: "transform 0.2s" }}
                >
                    +
                </button>
            </div>

            {/* Send Modal */}
            {showSendModal && (
                <div className="fixed inset-0 bg-gray-900 bg-opacity-75 flex items-center justify-center z-50 p-4">
                    <div className="bg-gray-800 rounded-xl p-8 shadow-2xl max-w-lg w-full border border-gray-700">
                        <h2 className="text-3xl font-bold text-white mb-6 text-center">Send Files To...</h2>
                        {connectedPeers.length === 0 ? (
                            <div className="text-gray-400 text-center p-4 bg-gray-700 rounded-lg">
                                <p className="mb-2">No connected peers found.</p>
                                <p>Connected peers will appear here once you establish a connection in the &quot;Peers&quot; view.</p>
                            </div>
                        ) : (
                            <div className="max-h-60 overflow-y-auto custom-scrollbar mb-6">
                                {connectedPeers.map(peer => (
                                    <label key={peer.instanceId} className="flex items-center p-3 bg-gray-700 rounded-lg mb-2 cursor-pointer hover:bg-gray-600 transition-colors duration-150">
                                        <input
                                            type="radio"
                                            name="peer-selection"
                                            checked={selectedPeerForSending?.instanceId === peer.instanceId}
                                            onChange={() => handlePeerSelectionChange(peer)}
                                            className="form-radio h-5 w-5 text-blue-600 bg-gray-900 border-gray-600 rounded focus:ring-blue-500"
                                        />
                                        <span className="ml-3 text-white font-medium">{peer.peerName}</span>
                                        <span className="ml-auto text-gray-400 text-sm">{peer.ipAddress}</span>
                                    </label>
                                ))}
                            </div>
                        )}

                        <div className="flex justify-end space-x-4 mt-4"> {/* Added mt-4 here */}
                            <button
                                onClick={handleCloseSendModal}
                                className="px-6 py-2 rounded-lg text-white font-bold bg-gray-700 hover:bg-gray-600 transition-colors duration-200 shadow-md"
                            >
                                Cancel
                            </button>
                            <button
                                onClick={handleConfirmSend}
                                className="modern-button px-6 py-2 rounded-lg text-white font-bold shadow-md"
                                disabled={!selectedPeerForSending} // Disable if no peer is selected
                            >
                                Confirm Send
                            </button>
                        </div>
                    </div>
                </div>
            )}
        </div>);
};

export default ExplorerView;