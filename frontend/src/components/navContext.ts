import { createContext, useContext } from 'react'
import type { NavGroup } from './navigation'

// The shell shares the navigation it built (permissions, role and module
// state) so pages such as the dashboard can offer the same entry points.
export const NavGroupsContext = createContext<NavGroup[]>([])
export const useNavGroups = () => useContext(NavGroupsContext)
