/*
Copyright (C) 2022-2023 Traefik Labs
This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.
You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/

import React from 'react'
import {
  NavigationDrawer,
  NavigationContainer,
  H3,
  H1,
  Flex,
  Link,
  NavigationTreeContainer,
  NavigationTreeItem as FaencyNavTreeItem,
} from '@traefiklabs/faency'
import { useLocation, useNavigate } from 'react-router-dom'
import { useAPIs } from 'hooks/use-apis'
// import { FiPower } from 'react-icons/fi'
import { FaFolder, FaFolderOpen, FaFileAlt } from 'react-icons/fa'
// import { useAuthDispatch, useAuthState } from 'context/auth'
// import { handleLogOut } from 'context/auth/actions'

// const CustomNavigationLink = NavigationLink as any

const NavigationTreeItem = ({
  name,
  type,
  children,
  specLink,
  ...props
}: {
  key: string
  name: string
  type: string
  children?: React.ReactNode
  specLink?: string
}) => {
  const { pathname } = useLocation()
  const navigate = useNavigate()

  return (
    <FaencyNavTreeItem
      active={pathname === specLink}
      onClick={() => navigate(specLink as string)}
      css={{ textAlign: 'justify', width: '100%' }}
      label={name}
      startAdornment={type === 'api' ? <FaFileAlt /> : null}
      {...props}
    >
      {children}
    </FaencyNavTreeItem>
  )
}

const SideNavbar = ({ portalName }: { portalName: string }) => {
  const { data: apis } = useAPIs()
  // const authDispatch = useAuthDispatch()
  // const { user } = useAuthState()

  const navigate = useNavigate()

  return (
    <NavigationDrawer css={{ width: 240 }}>
      <NavigationContainer
        css={{
          flexGrow: 1,
        }}
      >
        <>
          <Link
            onClick={() => navigate(`/`)}
            css={{ textDecoration: 'none', '&:hover': { textDecoration: 'none', cursor: 'pointer' } }}
          >
            <Flex css={{ height: '$10' }}>
              <H1>{portalName}</H1>
            </Flex>
          </Link>
          <H3>Available APIs</H3>
          <Flex direction="column" css={{ mt: '$5' }}>
            <NavigationTreeContainer defaultCollapseIcon={<FaFolderOpen />} defaultExpandIcon={<FaFolder />}>
              {apis?.collections?.map((collection, index: number) => (
                <NavigationTreeItem key={`sidenav-${index}`} name={collection.name} type="collection">
                  {collection.apis?.length &&
                    collection.apis.map((api: { name: string; specLink: string }, idx: number) => (
                      <NavigationTreeItem
                        key={`sidenav-${index}-${idx}`}
                        name={api.name}
                        specLink={api.specLink}
                        type="api"
                      />
                    ))}
                </NavigationTreeItem>
              ))}
            </NavigationTreeContainer>
            {apis?.apis?.map((api, index: number) => (
              <NavigationTreeItem key={`sidenav-api-${index}`} name={api.name} specLink={api.specLink} type="api" />
            ))}
          </Flex>
        </>
      </NavigationContainer>
      {/* <NavigationContainer>
        <Text css={{ pl: '$3', fontWeight: '500' }}>{user?.username}</Text>
        <CustomNavigationLink as="button" startAdornment={<FiPower />} onClick={() => handleLogOut(authDispatch)}>
          Log Out
        </CustomNavigationLink>
      </NavigationContainer> */}
    </NavigationDrawer>
  )
}

export default SideNavbar
