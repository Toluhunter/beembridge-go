import React from 'react';
import { IconType } from 'react-icons';
import { SlPeople } from "react-icons/sl";
import { BiTransfer } from "react-icons/bi";
import { LuFolders } from "react-icons/lu";
import { FaRegFolderOpen } from "react-icons/fa";
import { FaHistory } from "react-icons/fa";
import { CiSettings } from "react-icons/ci";
import { TbLayoutSidebarLeftCollapseFilled as CollapseIcon, TbLayoutSidebarRightCollapseFilled } from "react-icons/tb";

export interface SidebarItem {
    id: 'peers' | 'history' | 'active-transfers' | 'explorer' | 'settings';
    name: string;
    icon: IconType;
}

export const sidebarItems: SidebarItem[] = [
    { id: 'peers', name: 'Peers', icon: SlPeople },
    { id: 'history', name: 'Transfer History', icon: FaHistory },
    { id: 'active-transfers', name: 'Active Transfers', icon: BiTransfer }, // New item for active transfers
    { id: 'explorer', name: 'Explorer', icon: FaRegFolderOpen },
    { id: 'settings', name: 'Settings', icon: CiSettings }, // New settings item
];


interface SidebarProps {
    isSidebarCollapsed: boolean;
    isMouseOverSidebar: boolean;
    setIsMouseOverSidebar: (isOver: boolean) => void;
    toggleSidebar: () => void;
    activeView: SidebarItem['id'];
    setActiveView: (view: SidebarItem['id']) => void;
    userName: string;
    userId: string;
    logoBb: string;
}

export const Sidebar: React.FC<SidebarProps> = ({
    isSidebarCollapsed,
    isMouseOverSidebar,
    setIsMouseOverSidebar,
    toggleSidebar,
    activeView,
    setActiveView,
    userName,
    userId,
    logoBb,
}) => {
    return (
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
                                        className={`flex items-center w-full px-4 py-3 my-5 text-left transition-colors duration-200
                                    ${activeView === item.id
                                                ? 'text-white'
                                                : 'text-gray-400 hover:text-white'
                                            }
                                    ${activeView == item.id && isSidebarCollapsed ? 'rounded-none border-l-4 border-purple-700' : ''}
                                    ${activeView == item.id && !isSidebarCollapsed ? 'bg-purple-700 rounded-xl' : ''}
                                    ${isSidebarCollapsed ? 'justify-center px-2' : ''}`
                                        }
                                    >
                                        <span className={`text-2xl mr-3
                                            ${activeView === item.id ? 'text-white' : 'text-gray-400 group-hover:text-white'}`
                                        }><Icon className='stroke-1' /></span>
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
    );
};
