<?xml version="1.0" encoding="UTF-8"?>
<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:output method="xml" indent="yes"/>
  <xsl:template match="/">
    <coverage version="1">
      <xsl:for-each select="//class">
        <file>
          <xsl:attribute name="path">
            <xsl:value-of select="@filename"/>
          </xsl:attribute>
          <xsl:for-each select="lines/line">
            <lineToCover>
              <xsl:attribute name="lineNumber">
                <xsl:value-of select="@number"/>
              </xsl:attribute>
              <xsl:attribute name="covered">
                <xsl:choose>
                  <xsl:when test="@hits > 0">true</xsl:when>
                  <xsl:otherwise>false</xsl:otherwise>
                </xsl:choose>
              </xsl:attribute>
            </lineToCover>
          </xsl:for-each>
        </file>
      </xsl:for-each>
    </coverage>
  </xsl:template>
</xsl:stylesheet>
