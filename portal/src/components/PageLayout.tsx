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
import { Flex, Container, VariantProps, CSS } from '@traefiklabs/faency'
import { Helmet } from 'react-helmet-async'
import SideNavbar from 'components/SideNavbar'
import { getInjectedValues } from 'utils/getInjectedValues'

const { portalName } = getInjectedValues()

type Props = {
  title?: string
  children?: React.ReactNode
  noGutter?: boolean
  containerSize?: VariantProps<typeof Container>['size']
  maxWidth?: CSS['maxWidth']
  contentAlignment?: 'default' | 'left'
}

const PageLayout = ({
  children,
  title,
  noGutter = false,
  containerSize = '3',
  maxWidth,
  contentAlignment = 'default',
}: Props) => {
  return (
    <>
      <Helmet>
        <title>{title || 'API Portal'}</title>
      </Helmet>
      <Flex>
        <SideNavbar portalName={portalName as string} />
        <Flex direction="column" css={{ flex: 1, height: '100vh', overflowY: 'auto', position: 'relative', pb: '$3' }}>
          <Flex direction="column" css={{ flex: 1, pb: noGutter ? 0 : '$2', px: noGutter ? 0 : '$2' }}>
            <Container
              size={containerSize}
              noGutter={noGutter}
              css={{
                display: 'flex',
                maxWidth,
                flexDirection: 'column',
                width: '100%',
                flex: 1,
                mx: contentAlignment === 'left' ? 0 : 'auto',
              }}
            >
              <Flex direction="column" css={{ flex: 1 }}>
                {children}
              </Flex>
              {/* <Footer /> */}
            </Container>
          </Flex>
        </Flex>
      </Flex>
    </>
  )
}

export default PageLayout
