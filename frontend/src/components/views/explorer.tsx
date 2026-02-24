import React, { useState, useEffect } from "react";
import { DiscoveredPeer } from "./peers.js";
import { GetFileStats, OpenDirectoryDialog, OpenFileDialog } from "../../../wailsjs/go/main/App.js";
import { OnFileDrop, OnFileDropOff } from "../../../wailsjs/runtime/runtime.js";
import { main } from "../../../wailsjs/go/models.js";


interface ExplorerViewProps {
    selectedFiles: main.SelectedItem[];
    onAddFiles: (files: main.SelectedItem[]) => void;
    onRemoveFiles: (filesToRemove: main.SelectedItem[]) => void; // Updated to accept an array
    connectedPeers: DiscoveredPeer[]; // Added connectedPeers prop
    onSendFilesToPeers: (files: main.SelectedItem[], targetPeers: DiscoveredPeer[]) => void; // New prop for sending files
}

export const ExplorerView: React.FC<ExplorerViewProps> = ({ selectedFiles, onAddFiles, onRemoveFiles, connectedPeers, onSendFilesToPeers }) => {
    const [showSendModal, setShowSendModal] = useState(false);
    const [selectedPeerForSending, setSelectedPeerForSending] = useState<DiscoveredPeer | null>(null);
    const [showAddOptions, setShowAddOptions] = useState(false);
    const [selectedItemsForRemoval, setSelectedItemsForRemoval] = useState<Set<string>>(new Set()); // New state for checkboxes
    const [sortColumn, setSortColumn] = useState<keyof main.SelectedItem | null>('name');
    const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('asc');
    const [currentPage, setCurrentPage] = useState(1);
    const itemsPerPage = 5;

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

    const getType = (file: main.SelectedItem): string => {
        if (file.isDirectory) return 'Folder';
        const parts = file.name.split('.');
        if (parts.length > 1) {
            return parts[parts.length - 1].toUpperCase(); // Return extension in uppercase
        }
        return 'File';
    };

    const handleSort = (column: keyof main.SelectedItem | 'isDirectory') => {
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

    const totalSize = selectedFiles.reduce((acc, item) => acc + item.size, 0);
    const totalPages = Math.ceil(sortedFiles.length / itemsPerPage) || 1;
    const paginatedFiles = sortedFiles.slice((currentPage - 1) * itemsPerPage, currentPage * itemsPerPage);

    useEffect(() => {
        if (currentPage > totalPages) {
            setCurrentPage(totalPages);
        }
    }, [selectedFiles.length, totalPages, currentPage]);

    // Register Wails drag-and-drop handler for the window. When files are
    // dropped (and the drop is on an element that has the CSS drop property),
    // the OnFileDrop callback will provide absolute file paths which we turn
    // into SelectedItem objects via GetFileStats and add to the explorer.
    useEffect(() => {
        const cb = async (x: number, y: number, paths: string[]) => {
            try {
                if (paths && paths.length > 0) {
                    const files = await GetFileStats(paths);
                    onAddFiles(files);
                }
            } catch (err) {
                console.error('Error handling dropped files:', err);
            }
        };

        // useDropTarget = true so Wails will only call the callback when the
        // drop occurs on an element marked as a drop target (see CSS var below)
        OnFileDrop(cb, true);

        return () => {
            try {
                OnFileDropOff();
            } catch (err) {
                // Ignore errors during unmount cleanup
            }
        };
    }, [onAddFiles]);


    return (
        <div className="relative flex flex-col h-full p-8 shadow-lg overflow-y-none">
            <h1 className="text-4xl font-bold text-white mb-4 text-start">File Explorer</h1>
            <p className="text-gray-400 text-lg mb-4 text-start">Browse and manage your files for transfer.</p>

            <button
                onChange={handleOpenFile}
                className="hidden"
            />

            <div className="h-full p-4 pb-8 overflow-y-none">
                <div
                    className="rounded-lg flex flex-col items-center justify-center h-max-content text-gray-500 pb-6"
                    id="drop-zone"
                    // Mark this element as a Wails drop target using the CSS drop property
                    style={{ ["--wails-drop-target" as any]: "drop" } as React.CSSProperties}
                >
                    <div className="border border-dashed bg-gray-800 w-100 rounded-lg flex flex-col items-center justify-center h-max-content text-gray-500 p-4">
                        <svg className="w-16 h-16 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M9 13h6m-3-3v6m5 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"></path>
                        </svg>
                        <p className="text-sm">Drag and drop files and folders you want to upload here, or choose Add files or Add folder.</p>
                    </div>
                </div>

                <div className="border border-gray-700 rounded-lg shadow-md overflow-hidden">
                    <div className="flex justify-between items-center p-4 border-b border-gray-700 bg-gray-800/50">
                        <div className="text-white font-semibold">
                            Selected Files & Folders ({selectedFiles.length}) - {formatFileSize(totalSize)}
                        </div>
                        <div className="flex space-x-2 items-center">
                            <button
                                onClick={handleRemoveSelected}
                                disabled={selectedItemsForRemoval.size === 0}
                                className={
                                    `px-4 py-2 border rounded-lg text-sm font-semibold transition-colors shadow-sm ` +
                                    (selectedItemsForRemoval.size === 0
                                        ? 'border-gray-500 text-gray-500 hover:bg-transparent cursor-not-allowed'
                                        : 'border-red-600 text-white hover:bg-red-700')
                                }
                            >
                                Remove{selectedItemsForRemoval.size > 0 ? ` (${selectedItemsForRemoval.size})` : ''}
                            </button>
                            <button
                                onClick={handleAddFilesClick}
                                className="px-4 py-2 border hover:bg-blue-700 text-white rounded-lg text-sm font-semibold transition-colors shadow-sm"
                            >
                                Add Files
                            </button>
                            <button
                                onClick={handleAddDirectoryClick}
                                className="px-4 py-2 border hover:bg-green-700 text-white rounded-lg text-sm font-semibold transition-colors shadow-sm"
                            >
                                Add Folder
                            </button>
                        </div>
                    </div>
                    <table className="min-w-full divide-y divide-gray-700">
                        <thead className="bg-gray-700/50">
                            <tr className="p-0">
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
                            {paginatedFiles.length > 0 ? (
                                paginatedFiles.map((file) => (
                                    <tr key={file.path} className="hover:bg-gray-700/30 transition-colors">
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
                                ))
                            ) : (
                                <tr>
                                    <td colSpan={6} className="px-6 py-10 text-center text-gray-500">
                                        No files selected yet. Drag and drop files here or use the buttons above.
                                    </td>
                                </tr>
                            )}
                        </tbody>
                    </table>

                    <div className="flex justify-between items-center p-2 border-t border-gray-700 bg-gray-800/30">
                        <div className="text-sm text-gray-400">
                            {selectedFiles.length > 0 ? (
                                <>Showing {(currentPage - 1) * itemsPerPage + 1} to {Math.min(currentPage * itemsPerPage, selectedFiles.length)} of {selectedFiles.length} items</>
                            ) : (
                                <>0 items selected</>
                            )}
                        </div>
                        <div className="flex items-center space-x-4">
                            {totalPages > 1 && (
                                <div className="flex items-center space-x-2">
                                    <button
                                        onClick={() => setCurrentPage(prev => Math.max(prev - 1, 1))}
                                        disabled={currentPage === 1}
                                        className="px-2 py-1 text-gray-300 hover:text-white disabled:opacity-50 transition-colors"
                                        aria-label="Previous Page"
                                    >
                                        {'<'}
                                    </button>
                                    <div className="text-sm text-gray-300">{currentPage} / {totalPages}</div>
                                    <button
                                        onClick={() => setCurrentPage(prev => Math.min(prev + 1, totalPages))}
                                        disabled={currentPage === totalPages}
                                        className="px-2 py-1 text-gray-300 hover:text-white disabled:opacity-50 transition-colors"
                                        aria-label="Next Page"
                                    >
                                        {'>'}
                                    </button>
                                </div>
                            )}
                        </div>
                    </div>

                </div>

            </div>

            {/* Send Button - gradient border, no background; greyed when disabled */}
            <div className="absolute bottom-4 right-6">{/* container positioned bottom-right */}
                {selectedFiles.length > 0 ? (
                    <div className="rounded-lg p-[2px] border">
                        <button
                            onClick={handleSendClick}
                            // className="modern-button absolute bottom-0 right-14 px-4 py-2 rounded-lg text-lg font-bold shadow-lg transition-all duration-200 focus:outline-none hover:ring-4 hover:ring-blue-500 hover:ring-opacity-50"
                            className="bg-transparent px-4 py-2 rounded-md text-lg font-bold text-white transition-colors duration-200 focus:outline-none"
                            aria-label="Send Files"
                        >
                            Send
                        </button>
                    </div>
                ) : (
                    <div className="rounded-lg p-[2px] border border-gray-500">
                        <button
                            disabled
                            className="bg-transparent px-4 py-2 rounded-md text-lg font-bold text-gray-500 cursor-not-allowed border border-transparent"
                            aria-label="Send Files"
                        >
                            Send
                        </button>
                    </div>
                )}
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